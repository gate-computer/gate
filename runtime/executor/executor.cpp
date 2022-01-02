// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <cerrno>
#include <csignal>
#include <cstdint>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <memory>

#include <fcntl.h>
#include <sched.h>
#include <sys/epoll.h>
#include <sys/personality.h>
#include <sys/prctl.h>
#include <sys/resource.h>
#include <sys/socket.h>
#include <sys/stat.h>
#include <sys/syscall.h>
#include <sys/time.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <unistd.h>

#include "align.hpp"
#include "attribute.hpp"
#include "debug.hpp"
#include "errors.gen.h"
#include "runtime.hpp"

#ifndef CLONE_INTO_CGROUP
#define CLONE_INTO_CGROUP 0x200000000ULL
#endif

#define LOADER_FILENAME "gate-runtime-loader." GATE_COMPAT_VERSION

namespace {

struct CloneArgsV0 {
	uint64_t flags;       // Flags bit mask
	uint64_t pidfd;       // Where to store PID file descriptor (pid_t *)
	uint64_t child_tid;   // Where to store child TID, in child's memory (pid_t *)
	uint64_t parent_tid;  // Where to store child TID, in parent's memory (int *)
	uint64_t exit_signal; // Signal to deliver to parent on child termination
	uint64_t stack;       // Pointer to lowest byte of stack
	uint64_t stack_size;  // Size of stack
	uint64_t tls;         // Location of new TLS
};

struct CloneArgsV2 {
	CloneArgsV0 v0;
	uint64_t set_tid;      // Pointer to a pid_t array (Linux 5.5)
	uint64_t set_tid_size; // Number of elements in set_tid (Linux 5.5)
	uint64_t cgroup;       // File descriptor for target cgroup of child (Linux 5.7)
};

NORETURN void die(int code)
{
	debugf("executor: die with code %d", code);
	_exit(code);
}

// Close a file descriptor or die.
void xclose(int fd)
{
	if (close(fd) != 0)
		die(ERR_EXEC_CLOSE);
}

// Duplicate a file descriptor or die.
void xdup2(int oldfd, int newfd)
{
	if (dup2(oldfd, newfd) != newfd)
		die(ERR_EXECHILD_DUP2);
}

NORETURN int execute_child(const int io_fds[2])
{
	xdup2(io_fds[0], GATE_INPUT_FD);
	xdup2(io_fds[1], GATE_OUTPUT_FD);

	char* args[] = {LOADER_FILENAME, nullptr};
	char* none[] = {nullptr};

	syscall(SYS_execveat, GATE_LOADER_FD, "", args, none, AT_EMPTY_PATH);
	die(ERR_EXECHILD_EXEC_LOADER);
}

pid_t spawn_child(const int io_fds[2], int cgroup_fd, int* ret_pidfd)
{
	CloneArgsV2 args = {
		.v0 = {
			.flags = CLONE_PIDFD | CLONE_VFORK,
			.pidfd = uintptr_t(ret_pidfd),
			.child_tid = 0,
			.parent_tid = 0,
			.exit_signal = SIGCHLD,
			.stack = 0,
			.stack_size = 0,
			.tls = 0,
		},
		.set_tid = 0,
		.set_tid_size = 0,
		.cgroup = 0,
	};
	auto size = sizeof(CloneArgsV0);

	if (cgroup_fd >= 0) {
		args.v0.flags |= CLONE_INTO_CGROUP;
		args.cgroup = cgroup_fd;
		size = sizeof(CloneArgsV2);
	}

	pid_t pid = syscall(SYS_clone3, &args, size);
	if (pid == 0)
		execute_child(io_fds);

	return pid;
}

pid_t create_process(cmsghdr const* cmsg, int cgroup_fd, int* ret_pidfd)
{
	if (cmsg->cmsg_level != SOL_SOCKET)
		die(ERR_EXEC_CMSG_LEVEL);

	if (cmsg->cmsg_type != SCM_RIGHTS)
		die(ERR_EXEC_CMSG_TYPE);

	int num_fds;
	auto fds = reinterpret_cast<int const*>(CMSG_DATA(cmsg));

	if (cmsg->cmsg_len == CMSG_LEN(2 * sizeof(int))) {
		num_fds = 2;
	} else if (cmsg->cmsg_len == CMSG_LEN(3 * sizeof(int))) {
		num_fds = 3;
		cgroup_fd = fds[2];
	} else {
		die(ERR_EXEC_CMSG_LEN);
	}

	auto pid = spawn_child(fds, cgroup_fd, ret_pidfd);
	if (pid <= 0)
		die(ERR_EXEC_CLONE);

	for (int i = 0; i < num_fds; i++)
		xclose(fds[i]);

	return pid;
}

void signal_pidfd(int fd, int signum)
{
	if (syscall(SYS_pidfd_send_signal, fd, signum, 0, 0) != 0)
		die(ERR_EXEC_KILL);
}

// get_process_cpu_ticks returns -1 if the process is gone.
long get_process_cpu_ticks(pid_t pid)
{
	char name[16];
	snprintf(name, sizeof name, "%u/stat", pid);

	auto fd = openat(GATE_PROC_FD, name, O_RDONLY | O_CLOEXEC, 0);
	if (fd < 0) {
		if (errno == ENOENT) { // Already reaped.
			debugf("executor: pid %d stat file does not exist", pid);
			return -1;
		}

		die(ERR_EXEC_PROCSTAT_OPEN);
	}

	// The buffer is large enough for the first 15 tokens.
	char buf[512];
	auto len = read(fd, buf, sizeof buf - 1);
	if (len < 0)
		die(ERR_EXEC_PROCSTAT_READ);
	buf[len] = '\0';

	xclose(fd);

	// Find the end of the comm string.  It's the second token.
	auto s = strrchr(buf, ')');
	if (s == nullptr)
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

void suspend_process(pid_t pid, int pidfd, long clock_ticks)
{
	signal_pidfd(pidfd, SIGXCPU);

	auto spent_ticks = get_process_cpu_ticks(pid);
	if (spent_ticks < 0)
		return;

	// Add 1 second, rounding up.
	rlim_t secs = (spent_ticks + clock_ticks + clock_ticks / 2) / clock_ticks;

	debugf("executor: pid %d fd %d used %ld ticks -> limit %llu secs", pid, pidfd, spent_ticks, secs);

	const rlimit cpu = {
		.rlim_cur = secs,
		.rlim_max = secs,
	};

	if (prlimit(pid, RLIMIT_CPU, &cpu, nullptr) != 0)
		die(ERR_EXEC_PRLIMIT_CPU);
}

enum {
	EXEC_OP_CREATE,
	EXEC_OP_KILL,
	EXEC_OP_SUSPEND,
};

// See runtime/executor.go
struct ExecRequest {
	int16_t id;
	uint8_t op;
	uint8_t reserved[1];
} PACKED;

// See runtime/executor.go
struct ExecStatus {
	int16_t id;
	uint8_t reserved[2];
	int32_t status;
} PACKED;

union ControlBuffer {
	char buf[CMSG_SPACE(3 * sizeof(int))]; // Space for 3 file descriptors.
	cmsghdr alignment;
};

#define ID_PROCS 16384
#define ID_CONTROL -1

#define POLL_BUFLEN 128
#define RECEIVE_BUFLEN 128
#define SEND_BUFLEN 128

struct Process {
	pid_t pid; // 0 means nonexistent.
	int fd;    // Undefined if pid == 0.
};

class Executor {
	Executor(Executor const&) = delete;
	void operator=(Executor const&) = delete;

public:
	Executor() {}

	void init(long clock_ticks, int cgroup_fd);
	void execute();

private:
	void initiate_shutdown();
	void receive_ops();
	void send_queued();
	void wait_process(int16_t id);

	// Leave one slot unoccupied to distinguish between empty and full.
	int send_queue_avail() const { return (SEND_BUFLEN - 1) - send_queue_length(); }
	bool send_queue_empty() const { return m_send_beg == m_send_end; }
	int send_queue_length() const { return (m_send_end - m_send_beg) & (SEND_BUFLEN - 1); }

	long m_clock_ticks;
	int m_cgroup_fd;
	int m_epoll_fd;
	int m_proc_count;
	bool m_shutdown;
	bool m_recv_block;
	bool m_send_block;
	unsigned m_send_beg;
	unsigned m_send_end;

	epoll_event m_events[POLL_BUFLEN];

	ExecStatus m_send_buf[SEND_BUFLEN];

	// Receive buffers.
	mmsghdr m_msgs[RECEIVE_BUFLEN];
	iovec m_iovs[RECEIVE_BUFLEN];
	ExecRequest m_reqs[RECEIVE_BUFLEN];
	ControlBuffer m_ctls[RECEIVE_BUFLEN];

	Process m_id_procs[ID_PROCS];
};

void Executor::init(long clock_ticks, int cgroup_fd)
{
	m_clock_ticks = clock_ticks;
	m_cgroup_fd = cgroup_fd;

	m_epoll_fd = epoll_create1(EPOLL_CLOEXEC);
	if (m_epoll_fd < 0)
		die(ERR_EXEC_EPOLL_CREATE);

	epoll_event ev;
	ev.events = EPOLLIN | EPOLLOUT | EPOLLET;
	ev.data.u64 = ID_CONTROL;
	if (epoll_ctl(m_epoll_fd, EPOLL_CTL_ADD, GATE_CONTROL_FD, &ev) < 0)
		die(ERR_EXEC_EPOLL_ADD);

	for (int i = 0; i < RECEIVE_BUFLEN; i++) {
		m_iovs[i].iov_base = &m_reqs[i];
		m_iovs[i].iov_len = sizeof m_reqs[i];
		m_msgs[i].msg_hdr.msg_iov = &m_iovs[i];
		m_msgs[i].msg_hdr.msg_iovlen = 1;
		m_msgs[i].msg_hdr.msg_control = m_ctls[i].buf;
		m_msgs[i].msg_hdr.msg_controllen = sizeof m_ctls[i].buf;
	}
}

void Executor::initiate_shutdown()
{
	debugf("executor: shutdown initiated");

	m_shutdown = true;
	m_recv_block = true;

	epoll_event ev;
	ev.events = EPOLLOUT | EPOLLET;
	ev.data.u64 = ID_CONTROL;
	if (epoll_ctl(m_epoll_fd, EPOLL_CTL_MOD, GATE_CONTROL_FD, &ev) < 0)
		die(ERR_EXEC_EPOLL_MOD);
}

void Executor::receive_ops()
{
more:
	if (m_recv_block)
		return;

	auto count = recvmmsg(GATE_CONTROL_FD, m_msgs, RECEIVE_BUFLEN, MSG_CMSG_CLOEXEC | MSG_DONTWAIT, nullptr);
	if (count < 0) {
		if (errno == EAGAIN || errno == EWOULDBLOCK) {
			m_recv_block = true;
			return;
		}

		die(ERR_EXEC_RECVMMSG);
	}

	if (count == 0) {
		m_recv_block = true;
		return;
	}

	for (int i = 0; i < count; i++) {
		if (m_msgs[i].msg_len == 0) {
			initiate_shutdown();
			return;
		}

		if (m_msgs[i].msg_len != sizeof m_reqs[i])
			die(ERR_EXEC_MSG_LEN);

		if (m_msgs[i].msg_hdr.msg_flags & MSG_CTRUNC)
			die(ERR_EXEC_MSG_CTRUNC);

		auto id = m_reqs[i].id;
		if (id < 0 || id >= ID_PROCS)
			die(ERR_EXEC_ID_RANGE);

		auto p = &m_id_procs[id];
		auto cmsg = CMSG_FIRSTHDR(&m_msgs[i].msg_hdr);

		switch (m_reqs[i].op) {
		case EXEC_OP_CREATE:
			debugf("executor: creating [%d]", id);

			if (cmsg == nullptr)
				die(ERR_EXEC_CMSG_OP_MISMATCH);

			if (p->pid != 0)
				die(ERR_EXEC_CREATE_PROCESS_BAD_STATE);

			p->pid = create_process(cmsg, m_cgroup_fd, &p->fd);
			m_proc_count++;

			debugf("executor: created [%d] pid %d fd %d", id, p->pid, p->fd);

			epoll_event ev;
			ev.events = EPOLLIN;
			ev.data.u64 = id;
			if (epoll_ctl(m_epoll_fd, EPOLL_CTL_ADD, p->fd, &ev) < 0)
				die(ERR_EXEC_EPOLL_ADD);

			// Only one control message per ExecRequest.
			if (CMSG_NXTHDR(&m_msgs[i].msg_hdr, cmsg))
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
				suspend_process(p->pid, p->fd, m_clock_ticks);
				debugf("executor: suspended [%d] pid %d fd %d", id, p->pid, p->fd);
			} else {
				debugf("executor: [%d] does not exist", id);
			}
			break;

		default:
			die(ERR_EXEC_OP);
		}

		// Reset for next time.
		m_msgs[i].msg_hdr.msg_controllen = sizeof m_ctls[i].buf;
	}

	goto more;
}

void Executor::send_queued()
{
more:
	if (send_queue_empty())
		return;

	int flags;
	if (send_queue_avail() == 0) {
		debugf("executor: blocking on send");
		flags = 0;
	} else if (m_send_block) {
		return;
	} else {
		debugf("executor: nonblocking send");
		flags = MSG_DONTWAIT;
	}

	// pwritev2 doesn't support RWF_NOWAIT flag with socket.

	int num;
	if (m_send_beg < m_send_end)
		num = m_send_end - m_send_beg;
	else
		num = SEND_BUFLEN - m_send_beg;

	auto len = send(GATE_CONTROL_FD, &m_send_buf[m_send_beg], num * sizeof m_send_buf[0], flags);
	if (len < 0) {
		if (errno == EAGAIN || errno == EWOULDBLOCK) {
			m_send_block = true;
			return;
		}

		die(ERR_EXEC_SEND);
	}

	if (len == 0) {
		debugf("executor: immediate shutdown");
		die(0);
	}

	if (len & (sizeof m_send_buf[0] - 1))
		die(ERR_EXEC_SEND_ALIGN);

	unsigned count = len / sizeof m_send_buf[0];
	m_send_beg = (m_send_beg + count) & (SEND_BUFLEN - 1);

	debugf("executor: sent %u queued statuses (%d remain)", count, send_queue_length(x));

	goto more;
}

void Executor::wait_process(int16_t id)
{
	debugf("executor: waiting [%d]", id);

	auto p = &m_id_procs[id];
	if (p->pid == 0)
		die(ERR_EXEC_WAIT_PROCESS_BAD_STATE);

	int status;
	auto ret = waitpid(p->pid, &status, WNOHANG);
	if (ret == 0)
		return;
	if (ret != p->pid)
		die(ERR_EXEC_WAITPID);

	debugf("executor: reaped [%d] pid %d fd %d status 0x%x", id, p->pid, p->fd, status);

	if (epoll_ctl(m_epoll_fd, EPOLL_CTL_DEL, p->fd, nullptr) < 0)
		die(ERR_EXEC_EPOLL_DEL);

	xclose(p->fd);
	p->pid = 0;
	m_proc_count--;

	auto slot = &m_send_buf[m_send_end];
	slot->id = id;
	slot->status = status;
	m_send_end = (m_send_end + 1) & (SEND_BUFLEN - 1);

	debugf("executor: send queue length %d", send_queue_length(x));
}

void Executor::execute()
{
	while (!(m_shutdown && m_proc_count == 0 && send_queue_empty())) {
		send_queued();
		receive_ops();

		// Handling an event may allocate a slot in the send queue.
		auto buflen = send_queue_avail();
		if (buflen > POLL_BUFLEN)
			buflen = POLL_BUFLEN;

		auto count = epoll_wait(m_epoll_fd, m_events, buflen, -1);
		if (count < 0)
			die(ERR_EXEC_EPOLL_WAIT);

		for (int i = 0; i < count; i++) {
			auto ev = &m_events[i];

			if (ev->data.u64 < ID_PROCS) {
				wait_process(ev->data.u64);
			} else if (ev->data.u64 == uint64_t(ID_CONTROL)) {
				if (ev->events & EPOLLIN)
					m_recv_block = false;
				if (ev->events & EPOLLOUT)
					m_send_block = false;
				if (ev->events & EPOLLHUP)
					initiate_shutdown();
				if (ev->events & ~(EPOLLIN | EPOLLOUT | EPOLLHUP))
					die(ERR_EXEC_POLL_OTHER_EVENTS);
			} else {
				die(ERR_EXEC_POLL_OTHER_ID);
			}
		}
	}

	debugf("executor: shutdown complete");
}

int clear_caps()
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
	} data[2] = {
		{0, 0, 0},
		{0, 0, 0},
	};

	return syscall(SYS_capset, &header, data);
}

// Set close-on-exec flag on a file descriptor or die.
void set_cloexec(int fd)
{
	auto flags = fcntl(fd, F_GETFD);
	if (flags < 0)
		die(ERR_EXEC_FCNTL_GETFD);

	if (fcntl(fd, F_SETFD, flags | FD_CLOEXEC) < 0)
		die(ERR_EXEC_FCNTL_CLOEXEC);
}

// Increase program break or die.  Constructor T is invoked.
template <typename T>
T* xbrk(long pagesize)
{
	auto size = align_size(sizeof(T), pagesize);

	// musl doesn't support sbrk at all; use brk directly.
	unsigned long begin = syscall(SYS_brk, 0);
	unsigned long end = syscall(SYS_brk, begin + size);
	if (end != begin + size)
		die(ERR_EXEC_BRK);

	auto ptr = reinterpret_cast<T*>(begin);
	new (ptr) T;
	return ptr;
}

// Set a resource limit or die.
void xsetrlimit(int resource, rlim_t limit, int exitcode)
{
	const rlimit buf = {
		.rlim_cur = limit,
		.rlim_max = limit,
	};

	if (setrlimit(resource, &buf) != 0)
		die(exitcode);
}

} // namespace

// Stdio, runtime, epoll, exec request, child dups, pidfs.
#define NOFILE (3 + 4 + 1 + 3 + 2 + ID_PROCS)

int main(int argc, char** argv)
{
	if (argc == 2 && strcmp(argv[1], "--compat") == 0)
		return puts("Interface version " GATE_COMPAT_VERSION) == EOF;

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
	set_cloexec(GATE_CGROUP_FD);
	set_cloexec(GATE_PROC_FD);

	int cgroup_fd = GATE_CGROUP_FD;
	struct stat st;
	if (fstat(cgroup_fd, &st) != 0)
		die(ERR_EXEC_FSTAT);
	if (S_ISCHR(st.st_mode)) // It might be /dev/null.
		cgroup_fd = -1;

	if (GATE_SANDBOX) {
		if (prctl(PR_SET_DUMPABLE, 0) != 0)
			die(ERR_EXEC_PRCTL_NOT_DUMPABLE);
	}

	sigset_t sigmask;
	sigemptyset(&sigmask);
	sigaddset(&sigmask, SIGCHLD);
	if (sigprocmask(SIG_SETMASK, &sigmask, nullptr) != 0)
		die(ERR_EXEC_SIGMASK);

	auto clock_ticks = sysconf(_SC_CLK_TCK);
	if (clock_ticks <= 0)
		die(ERR_EXEC_SYSCONF_CLK_TCK);

	auto pagesize = sysconf(_SC_PAGESIZE);
	if (pagesize <= 0)
		die(ERR_EXEC_PAGESIZE);

	auto x = xbrk<Executor>(pagesize);
	x->init(clock_ticks, cgroup_fd);

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

	x->execute();
	return 0;
}
