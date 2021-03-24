// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#define _GNU_SOURCE

#include <errno.h>
#include <sched.h>
#include <signal.h>
#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include <fcntl.h>
#include <sys/epoll.h>
#include <sys/personality.h>
#include <sys/prctl.h>
#include <sys/resource.h>
#include <sys/socket.h>
#include <sys/syscall.h>
#include <sys/time.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <unistd.h>

#include "align.h"
#include "attribute.h"
#include "debug.h"
#include "errors.h"
#include "runtime.h"

NORETURN
static void die(int code)
{
	debugf("executor: die with code %d", code);
	_exit(code);
}

// Close a file descriptor or die.
static void xclose(int fd)
{
	if (close(fd) != 0)
		die(ERR_EXEC_CLOSE);
}

// Duplicate a file descriptor or die.
static void xdup2(int oldfd, int newfd)
{
	if (dup2(oldfd, newfd) != newfd)
		die(ERR_EXECHILD_DUP2);
}

NORETURN
static int execute_child(void *args)
{
	const int *fds = args;

	xdup2(fds[0], GATE_INPUT_FD);
	xdup2(fds[1], GATE_OUTPUT_FD);

	char *none[] = {NULL};

	syscall(SYS_execveat, GATE_LOADER_FD, "", none, none, AT_EMPTY_PATH);
	die(ERR_EXECHILD_EXEC_LOADER);
}

static pid_t spawn_child(int *fds, int *pidfd)
{
	union {
		char buf[4096];
		__int128 align;
	} stack;

	void *stacktop = stack.buf + sizeof stack.buf;

	return clone(execute_child, stacktop, CLONE_PIDFD | CLONE_VFORK | SIGCHLD, fds, pidfd);
}

static pid_t create_process(struct cmsghdr *cmsg, int *pidfd)
{
	if (cmsg->cmsg_level != SOL_SOCKET)
		die(ERR_EXEC_CMSG_LEVEL);

	if (cmsg->cmsg_type != SCM_RIGHTS)
		die(ERR_EXEC_CMSG_TYPE);

	if (cmsg->cmsg_len != CMSG_LEN(2 * sizeof(int)))
		die(ERR_EXEC_CMSG_LEN);

	int *fds = (int *) CMSG_DATA(cmsg);

	*pidfd = -1;
	pid_t pid = spawn_child(fds, pidfd);
	if (pid <= 0)
		die(ERR_EXEC_CLONE);

	xclose(fds[1]);
	xclose(fds[0]);

	return pid;
}

static void signal_pidfd(int fd, int signum)
{
	if (syscall(SYS_pidfd_send_signal, fd, signum, 0, 0) != 0)
		die(ERR_EXEC_KILL);
}

// get_process_cpu_ticks returns -1 if the process is gone.
static long get_process_cpu_ticks(pid_t pid)
{
	char name[16];
	snprintf(name, sizeof name, "%u/stat", pid);

	int fd = openat(GATE_PROC_FD, name, O_RDONLY | O_CLOEXEC, 0);
	if (fd < 0) {
		if (errno == ENOENT) { // Already reaped.
			debugf("executor: pid %d stat file does not exist", pid);
			return -1;
		}

		die(ERR_EXEC_PROCSTAT_OPEN);
	}

	// The buffer is large enough for the first 15 tokens.
	char buf[512];
	ssize_t len = read(fd, buf, sizeof buf - 1);
	if (len < 0)
		die(ERR_EXEC_PROCSTAT_READ);
	buf[len] = '\0';

	xclose(fd);

	// Find the end of the comm string.  It's the second token.
	const char *s = strrchr(buf, ')');
	if (s == NULL)
		die(ERR_EXEC_PROCSTAT_PARSE);

	char state = '\0';
	unsigned long utime = 0;
	unsigned long stime = 0;

	//             2  3   4   5   6   7   8   9  10  11  12  13  14  15
	if (sscanf(s, ") %c %*d %*d %*d %*d %*d %*d %*u %*u %*u %*u %lu %lu ", &state, &utime, &stime) != 3)
		die(ERR_EXEC_PROCSTAT_PARSE);

	debugf("executor: pid %d state is %c", pid, state);

	switch (state) {
	case 'Z': // Zombie
	case 'X': // Dead
		return -1;
	}

	return utime + stime;
}

static void suspend_process(pid_t pid, int pidfd, long clock_ticks)
{
	signal_pidfd(pidfd, SIGXCPU);

	long spent_ticks = get_process_cpu_ticks(pid);
	if (spent_ticks < 0)
		return;

	// Add 1 second, rounding up.
	long secs = (spent_ticks + clock_ticks + clock_ticks / 2) / clock_ticks;

	debugf("executor: pid %d fd %d used %ld ticks -> limit %ld secs", pid, pidfd, spent_ticks, secs);

	const struct rlimit cpu = {
		.rlim_cur = secs,
		.rlim_max = secs,
	};

	if (prlimit(pid, RLIMIT_CPU, &cpu, NULL) != 0) {
		// if (errno == ESRCH) {
		// 	debugf("executor: pid %d fd %d does not exist (suspend)", pid, pidfd);
		// 	return;
		// }

		die(ERR_EXEC_PRLIMIT_CPU);
	}
}

enum {
	EXEC_OP_CREATE,
	EXEC_OP_KILL,
	EXEC_OP_SUSPEND,
};

// See runtime/executor.go
struct exec_request {
	int16_t id;
	uint8_t op;
	uint8_t reserved[1];
} PACKED;

// See runtime/executor.go
struct exec_status {
	int16_t id;
	uint8_t reserved[2];
	int32_t status;
} PACKED;

union control_buffer {
	char buf[CMSG_SPACE(2 * sizeof(int))]; // Space for 2 file descriptors.
	struct cmsghdr alignment;
};

#define ID_PROCS 16384
#define ID_CONTROL -1

#define POLL_BUFLEN 128
#define RECEIVE_BUFLEN 128
#define SEND_BUFLEN 128

struct process {
	pid_t pid; // 0 means nonexistent.
	int fd;    // Undefined if pid == 0.
};

struct executor {
	long clock_ticks;
	int epoll_fd;
	int proc_count;
	bool shutdown;
	bool recv_block;
	bool send_block;
	unsigned send_beg;
	unsigned send_end;

	struct epoll_event events[POLL_BUFLEN];

	struct exec_status send_buf[SEND_BUFLEN];

	// Receive buffers
	struct mmsghdr msgs[RECEIVE_BUFLEN];
	struct iovec iovs[RECEIVE_BUFLEN];
	struct exec_request reqs[RECEIVE_BUFLEN];
	union control_buffer ctls[RECEIVE_BUFLEN];

	struct process id_procs[ID_PROCS];
};

static void init_executor(struct executor *x, long clock_ticks)
{
	x->clock_ticks = clock_ticks;

	x->epoll_fd = epoll_create1(EPOLL_CLOEXEC);
	if (x->epoll_fd < 0)
		die(ERR_EXEC_EPOLL_CREATE);

	struct epoll_event ev;
	ev.events = EPOLLIN | EPOLLOUT | EPOLLET;
	ev.data.u64 = ID_CONTROL;
	if (epoll_ctl(x->epoll_fd, EPOLL_CTL_ADD, GATE_CONTROL_FD, &ev) < 0)
		die(ERR_EXEC_EPOLL_ADD);

	for (int i = 0; i < RECEIVE_BUFLEN; i++) {
		x->iovs[i].iov_base = &x->reqs[i];
		x->iovs[i].iov_len = sizeof x->reqs[i];
		x->msgs[i].msg_hdr.msg_iov = &x->iovs[i];
		x->msgs[i].msg_hdr.msg_iovlen = 1;
		x->msgs[i].msg_hdr.msg_control = x->ctls[i].buf;
		x->msgs[i].msg_hdr.msg_controllen = sizeof x->ctls[i].buf;
	}
}

static void initiate_shutdown(struct executor *x)
{
	debugf("executor: shutdown initiated");

	x->shutdown = true;
	x->recv_block = true;

	struct epoll_event ev;
	ev.events = EPOLLOUT | EPOLLET;
	ev.data.u64 = ID_CONTROL;
	if (epoll_ctl(x->epoll_fd, EPOLL_CTL_MOD, GATE_CONTROL_FD, &ev) < 0)
		die(ERR_EXEC_EPOLL_MOD);
}

static void receive_ops(struct executor *x)
{
more:
	if (x->recv_block)
		return;

	int count = recvmmsg(GATE_CONTROL_FD, x->msgs, RECEIVE_BUFLEN, MSG_CMSG_CLOEXEC | MSG_DONTWAIT, NULL);
	if (count < 0) {
		if (errno == EAGAIN || errno == EWOULDBLOCK) {
			x->recv_block = true;
			return;
		}

		die(ERR_EXEC_RECVMMSG);
	}

	if (count == 0) {
		x->recv_block = true;
		return;
	}

	for (int i = 0; i < count; i++) {
		if (x->msgs[i].msg_len == 0) {
			initiate_shutdown(x);
			return;
		}

		if (x->msgs[i].msg_len != sizeof x->reqs[i])
			die(ERR_EXEC_MSG_LEN);

		if (x->msgs[i].msg_hdr.msg_flags & MSG_CTRUNC)
			die(ERR_EXEC_MSG_CTRUNC);

		int16_t id = x->reqs[i].id;
		if (id < 0 || id >= ID_PROCS)
			die(ERR_EXEC_ID_RANGE);

		struct process *p = &x->id_procs[id];
		struct cmsghdr *cmsg = CMSG_FIRSTHDR(&x->msgs[i].msg_hdr);

		switch (x->reqs[i].op) {
		case EXEC_OP_CREATE:
			debugf("executor: creating [%d]", id);

			if (cmsg == NULL)
				die(ERR_EXEC_CMSG_OP_MISMATCH);

			if (p->pid != 0)
				die(ERR_EXEC_CREATE_PROCESS_BAD_STATE);

			p->pid = create_process(cmsg, &p->fd);
			x->proc_count++;

			debugf("executor: created [%d] pid %d fd %d", id, p->pid, p->fd);

			struct epoll_event ev;
			ev.events = EPOLLIN;
			ev.data.u64 = id;
			if (epoll_ctl(x->epoll_fd, EPOLL_CTL_ADD, p->fd, &ev) < 0)
				die(ERR_EXEC_EPOLL_ADD);

			// Only one control message per exec_request.
			if (CMSG_NXTHDR(&x->msgs[i].msg_hdr, cmsg))
				die(ERR_EXEC_CMSG_NXTHDR);
			break;

		case EXEC_OP_KILL:
			debugf("executor: killing [%d]", id);

			if (cmsg)
				die(ERR_EXEC_CMSG_OP_MISMATCH);

			if (p->pid != 0) {
				signal_pidfd(p->fd, SIGKILL);
				debugf("executor: killed [%d] pid %d fd %d", id, p->pid, p->fd);
			} else {
				debugf("executor: [%d] does not exist", id);
			}
			break;

		case EXEC_OP_SUSPEND:
			debugf("executor: suspending [%d]", id);

			if (cmsg)
				die(ERR_EXEC_CMSG_OP_MISMATCH);

			if (p->pid != 0) {
				suspend_process(p->pid, p->fd, x->clock_ticks);
				debugf("executor: suspended [%d] pid %d fd %d", id, p->pid, p->fd);
			} else {
				debugf("executor: [%d] does not exist", id);
			}
			break;

		default:
			die(ERR_EXEC_OP);
		}

		// Reset for next time.
		x->msgs[i].msg_hdr.msg_controllen = sizeof x->ctls[i].buf;
	}

	goto more;
}

static bool send_queue_empty(const struct executor *x)
{
	return x->send_beg == x->send_end;
}

static int send_queue_length(const struct executor *x)
{
	return (x->send_end - x->send_beg) & (SEND_BUFLEN - 1);
}

static int send_queue_avail(const struct executor *x)
{
	// Leave one slot unoccupied to distinguish between empty and full.
	return (SEND_BUFLEN - 1) - send_queue_length(x);
}

static void send_queued(struct executor *x)
{
more:
	if (send_queue_empty(x))
		return;

	int flags;
	if (send_queue_avail(x) == 0) {
		debugf("executor: blocking on send");
		flags = 0;
	} else if (x->send_block) {
		return;
	} else {
		debugf("executor: nonblocking send");
		flags = MSG_DONTWAIT;
	}

	// pwritev2 doesn't support RWF_NOWAIT flag with socket.

	int num;
	if (x->send_beg < x->send_end)
		num = x->send_end - x->send_beg;
	else
		num = SEND_BUFLEN - x->send_beg;

	ssize_t len = send(GATE_CONTROL_FD, &x->send_buf[x->send_beg], num * sizeof x->send_buf[0], flags);
	if (len < 0) {
		if (errno == EAGAIN || errno == EWOULDBLOCK) {
			x->send_block = true;
			return;
		}

		die(ERR_EXEC_SEND);
	}

	if (len == 0) {
		debugf("executor: immediate shutdown");
		die(0);
	}

	if (len & (sizeof x->send_buf[0] - 1))
		die(ERR_EXEC_SEND_ALIGN);

	unsigned count = len / sizeof x->send_buf[0];
	x->send_beg = (x->send_beg + count) & (SEND_BUFLEN - 1);

	debugf("executor: sent %u queued statuses (%d remain)", count, send_queue_length(x));

	goto more;
}

static void wait_process(struct executor *x, int16_t id)
{
	struct process *p = &x->id_procs[id];
	if (p->pid == 0)
		die(ERR_EXEC_WAIT_PROCESS_BAD_STATE);

	int status;
	pid_t ret = waitpid(p->pid, &status, WNOHANG);
	if (ret == 0)
		return;
	if (ret != p->pid)
		die(ERR_EXEC_WAITPID);

	debugf("executor: reaped [%d] pid %d fd %d status 0x%x", id, p->pid, p->fd, status);

	xclose(p->fd);
	p->pid = 0;
	x->proc_count--;

	struct exec_status *slot = &x->send_buf[x->send_end];
	slot->id = id;
	slot->status = status;
	x->send_end = (x->send_end + 1) & (SEND_BUFLEN - 1);

	debugf("executor: send queue length %d", send_queue_length(x));
}

static void executor(struct executor *x)
{
	while (!(x->shutdown && x->proc_count == 0 && send_queue_empty(x))) {
		send_queued(x);
		receive_ops(x);

		// Handling an event may allocate a slot in the send queue.
		int buflen = send_queue_avail(x);
		if (buflen > POLL_BUFLEN)
			buflen = POLL_BUFLEN;

		int count = epoll_wait(x->epoll_fd, x->events, buflen, -1);
		if (count < 0)
			die(ERR_EXEC_EPOLL_WAIT);

		for (int i = 0; i < count; i++) {
			const struct epoll_event *ev = &x->events[i];

			if (ev->data.u64 < ID_PROCS) {
				wait_process(x, ev->data.u64);
			} else if (ev->data.u64 == (uint64_t) ID_CONTROL) {
				if (ev->events & EPOLLIN)
					x->recv_block = false;
				if (ev->events & EPOLLOUT)
					x->send_block = false;
				if (ev->events & EPOLLHUP)
					initiate_shutdown(x);
				if (ev->events & ~(EPOLLIN | EPOLLOUT | EPOLLHUP))
					die(ERR_EXEC_POLL_OTHER_EVENTS);
			} else {
				die(ERR_EXEC_POLL_OTHER_ID);
			}
		}
	}

	debugf("executor: shutdown complete");
}

static inline int clear_caps(void)
{
	struct {
		uint32_t version;
		int pid;
	} header = {
		.version = 0x20080522, // ABI version 3.
		.pid = 0,
	};

	const struct {
		uint32_t effective, permitted, inheritable;
	} data[2] = {{0}, {0}};

	return syscall(SYS_capset, &header, data);
}

// Set close-on-exec flag on a file descriptor or die.
static void set_cloexec(int fd)
{
	int flags = fcntl(fd, F_GETFD);
	if (flags < 0)
		die(ERR_EXEC_FCNTL_GETFD);

	if (fcntl(fd, F_SETFD, flags | FD_CLOEXEC) < 0)
		die(ERR_EXEC_FCNTL_CLOEXEC);
}

// Increase program break or die.
static void *xbrk(size_t size, long pagesize)
{
	size = align_size(size, pagesize);

	// musl doesn't support sbrk at all; use brk directly.
	unsigned long begin = syscall(SYS_brk, 0);
	unsigned long end = syscall(SYS_brk, begin + size);
	if (end != begin + size)
		die(ERR_EXEC_BRK);

	return (void *) begin;
}

// Set a resource limit or die.
static void xsetrlimit(int resource, rlim_t limit, int exitcode)
{
	const struct rlimit buf = {
		.rlim_cur = limit,
		.rlim_max = limit,
	};

	if (setrlimit(resource, &buf) != 0)
		die(exitcode);
}

// Stdio, runtime, epoll, exec request input/output, child dups, pidfs.
#define NOFILE (3 + 3 + 1 + 2 + 2 + ID_PROCS)

int main(int argc, char **argv)
{
	if (prctl(PR_SET_PDEATHSIG, SIGKILL) != 0)
		die(ERR_EXEC_PDEATHSIG);

	if (prctl(PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0) != 0)
		die(ERR_EXEC_NO_NEW_PRIVS);

	if (clear_caps() != 0)
		die(ERR_EXEC_CLEAR_CAPS);

	set_cloexec(STDIN_FILENO);
	set_cloexec(STDOUT_FILENO);
	set_cloexec(STDERR_FILENO);
	set_cloexec(GATE_CONTROL_FD);
	set_cloexec(GATE_LOADER_FD);
	set_cloexec(GATE_PROC_FD);

	if (GATE_SANDBOX) {
		if (prctl(PR_SET_DUMPABLE, 0) != 0)
			die(ERR_EXEC_PRCTL_NOT_DUMPABLE);
	}

	sigset_t sigmask;
	sigemptyset(&sigmask);
	sigaddset(&sigmask, SIGCHLD);
	if (sigprocmask(SIG_SETMASK, &sigmask, NULL) != 0)
		die(ERR_EXEC_SIGMASK);

	long clock_ticks = sysconf(_SC_CLK_TCK);
	if (clock_ticks <= 0)
		die(ERR_EXEC_SYSCONF_CLK_TCK);

	long pagesize = sysconf(_SC_PAGESIZE);
	if (pagesize <= 0)
		die(ERR_EXEC_PAGESIZE);

	struct executor *x = xbrk(sizeof(struct executor), pagesize);
	init_executor(x, clock_ticks);

	if (GATE_SANDBOX) {
		xsetrlimit(RLIMIT_DATA, GATE_LIMIT_DATA, ERR_EXEC_SETRLIMIT_DATA);
		xsetrlimit(RLIMIT_STACK, align_size(GATE_LOADER_STACK_SIZE, pagesize), ERR_EXEC_SETRLIMIT_STACK);
	}

	xsetrlimit(RLIMIT_NOFILE, NOFILE, ERR_EXEC_SETRLIMIT_NOFILE);

	// ASLR makes stack size and stack pointer position unpredictable, so
	// it's hard to unmap the initial stack in loader.  Run-time mapping
	// addresses are randomized manually anyway.
	if (personality(ADDR_NO_RANDOMIZE) < 0)
		die(ERR_EXEC_PERSONALITY_ADDR_NO_RANDOMIZE);

	executor(x);
	return 0;
}
