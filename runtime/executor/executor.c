// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#define _GNU_SOURCE

#include <assert.h>
#include <errno.h>
#include <signal.h>
#include <stdbool.h>
#include <stddef.h>
#include <stdlib.h>
#include <string.h>

#include <fcntl.h>
#include <poll.h>
#include <sys/personality.h>
#include <sys/prctl.h>
#include <sys/resource.h>
#include <sys/socket.h>
#include <sys/time.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <unistd.h>

#include "buffer.h"
#include "errors.h"
#include "execveat.h"
#include "runtime.h"

#define NOINLINE __attribute__((noinline))
#define NORETURN __attribute__((noreturn))
#define PACKED __attribute__((packed))

#define CHILD_NICE 19

#define SENDING_CAPACITY (BUFFER_MAX_ENTRIES * sizeof(struct send_entry))
#define KILLED_CAPACITY (BUFFER_MAX_ENTRIES * sizeof(pid_t))
#define DIED_CAPACITY (BUFFER_MAX_ENTRIES * sizeof(pid_t))

// runtime/executor.go relies on this assumption
static_assert(sizeof(pid_t) == sizeof(int32_t), "pid_t size");

// send_entry is like recvEntry in runtime/executor.go
struct send_entry {
	pid_t pid;      // Negated value acknowledges execution request.
	int32_t status; // Defined only if pid is positive.
} PACKED;

// Duplicate a file descriptor or die.
static void xdup2(int oldfd, int newfd)
{
	if (dup2(oldfd, newfd) != newfd)
		_exit(ERR_EXECHILD_DUP2);
}

// Set a resource limit or die.
static void xlimit(int resource, rlim_t rlim, int exitcode)
{
	struct rlimit buf = {
		.rlim_cur = rlim,
		.rlim_max = rlim,
	};

	if (setrlimit(resource, &buf) != 0)
		_exit(exitcode);
}

static void sandbox_child(void)
{
	xlimit(RLIMIT_NOFILE, 0, ERR_EXECHILD_SETRLIMIT_NOFILE);
	xlimit(RLIMIT_NPROC, 0, ERR_EXECHILD_SETRLIMIT_NPROC);

	if (prctl(PR_SET_TSC, PR_TSC_SIGSEGV, 0, 0, 0) != 0)
		_exit(ERR_EXECHILD_PRCTL_TSC_SIGSEGV);
}

NORETURN
static inline void execute_child(const int *fds, int num_fds)
{
	xdup2(fds[0], GATE_INPUT_FD);
	xdup2(fds[1], GATE_OUTPUT_FD);
	xdup2(fds[2], GATE_IMAGE_FD);

	if (num_fds > 3)
		xdup2(fds[3], GATE_DEBUG_FD);

	if (nice(CHILD_NICE) != CHILD_NICE)
		_exit(ERR_EXECHILD_NICE);

	if (GATE_SANDBOX)
		sandbox_child();

	// ASLR makes stack size and stack pointer position unpredictable, so
	// it's hard to unmap the initial stack.  Run-time mapping addresses
	// are randomized manually.
	if (personality(ADDR_NO_RANDOMIZE) < 0)
		_exit(ERR_EXECHILD_PERSONALITY_ADDR_NO_RANDOMIZE);

	char *none[] = {NULL};

	sys_execveat(GATE_LOADER_FD, "", none, none, AT_EMPTY_PATH);
	_exit(ERR_EXECHILD_EXEC_LOADER);
}

NOINLINE
static pid_t spawn_child(const int *fds, int num_fds)
{
	pid_t pid = vfork();
	if (pid == 0)
		execute_child(fds, num_fds);

	return pid;
}

// Set close-on-exec flag on a file descriptor or die.
static void xcloexec(int fd)
{
	int flags = fcntl(fd, F_GETFD);
	if (flags < 0)
		_exit(ERR_EXEC_FCNTL_GETFD);

	if (fcntl(fd, F_SETFD, flags | FD_CLOEXEC) < 0)
		_exit(ERR_EXEC_FCNTL_CLOEXEC);
}

static void sighandler(int signum)
{
}

static void init_sigchld()
{
	sigset_t mask;
	sigemptyset(&mask);
	sigaddset(&mask, SIGCHLD);
	if (sigprocmask(SIG_SETMASK, &mask, NULL) != 0)
		_exit(ERR_EXEC_SIGPROCMASK);

	if (signal(SIGCHLD, sighandler) == SIG_ERR)
		_exit(ERR_EXEC_SIGNAL_HANDLER);
}

// Set up polling and signal mask according to buffer status and return revents
// by value.
static inline int do_ppoll(struct buffer *sending, struct buffer *killed, struct buffer *died)
{
	struct pollfd pollfd = {
		.fd = GATE_CONTROL_FD,
	};

	sigset_t pollmask_storage;
	sigset_t *pollmask = NULL;

	if (buffer_space(sending, sizeof(struct send_entry))) {
		if (buffer_space_pid(killed))
			pollfd.events |= POLLIN;

		if (buffer_space_pid(died)) {
			// Enable reaping.
			pollmask = &pollmask_storage;
			sigemptyset(pollmask);
		}
	}

	if (buffer_content(sending))
		pollfd.events |= POLLOUT;

	int count = ppoll(&pollfd, 1, NULL, pollmask); // May invoke sighandler.
	if (count < 0) {
		if (errno == EINTR)
			return -1; // Reap.

		_exit(ERR_EXEC_PPOLL);
	}

	return (unsigned short) pollfd.revents; // Zero-extension.
}

static inline ssize_t do_recvmsg(struct msghdr *msg, void *buf, size_t buflen, int flags)
{
	struct iovec io = {
		.iov_base = buf,
		.iov_len = buflen,
	};

	msg->msg_iov = &io;
	msg->msg_iovlen = 1;

	return recvmsg(GATE_CONTROL_FD, msg, flags);
}

static inline void handle_control_message(struct buffer *sending, struct cmsghdr *cmsg)
{
	if (cmsg->cmsg_level != SOL_SOCKET)
		_exit(ERR_EXEC_CMSG_LEVEL);

	if (cmsg->cmsg_type != SCM_RIGHTS)
		_exit(ERR_EXEC_CMSG_TYPE);

	int num_fds;
	if (cmsg->cmsg_len == CMSG_LEN(3 * sizeof(int)))
		num_fds = 3;
	else if (cmsg->cmsg_len == CMSG_LEN(4 * sizeof(int)))
		num_fds = 4;
	else
		_exit(ERR_EXEC_CMSG_LEN);

	const int *fds = (int *) CMSG_DATA(cmsg);

	pid_t pid = spawn_child(fds, num_fds);
	if (pid <= 0)
		_exit(ERR_EXEC_VFORK);

	for (int i = 0; i < num_fds; i++)
		close(fds[i]);

	const struct send_entry entry = {-pid, 0};
	if (buffer_append(sending, &entry, sizeof entry) != 0)
		_exit(ERR_EXEC_SENDBUF_OVERFLOW_CMSG);
}

static inline void handle_pid_message(struct buffer *killed, struct buffer *died, pid_t pid)
{
	int signum = SIGKILL;

	if (pid < 0) {
		pid = -pid;
		signum = SIGXCPU;
	}

	if (pid == 1)
		_exit(ERR_EXEC_KILLMSG_PID);

	if (!buffer_remove_pid(died, pid)) {
		if (kill(pid, signum) != 0)
			_exit(ERR_EXEC_KILL);

		if (signum == SIGXCPU) {
			struct rlimit buf = {
				.rlim_cur = 1,
				.rlim_max = 1, // SIGKILL in one second.
			};

			if (prlimit(pid, RLIMIT_CPU, &buf, NULL) != 0)
				_exit(ERR_EXEC_PRLIMIT);
		}

		if (buffer_append_pid(killed, pid) != 0)
			_exit(ERR_EXEC_KILLBUF_OVERFLOW);
	}
}

static inline void handle_receiving(struct buffer *sending, struct buffer *killed, struct buffer *died)
{
	union {
		char buf[sizeof(pid_t)];
		pid_t pid;
	} receive;

	for (size_t receive_len = 0; receive_len < sizeof receive.buf;) {
		union {
			char buf[CMSG_SPACE(4 * sizeof(int))];
			struct cmsghdr alignment;
		} ctl;

		struct msghdr msg = {
			.msg_control = ctl.buf,
			.msg_controllen = sizeof ctl.buf,
		};

		int flags = MSG_CMSG_CLOEXEC;
		if (receive_len == 0)
			flags |= MSG_DONTWAIT;

		ssize_t len = do_recvmsg(&msg, receive.buf + receive_len, sizeof receive.buf - receive_len, flags);
		if (len <= 0) {
			if (len == 0)
				_exit(0);

			if (receive_len == 0) {
				if (errno == EAGAIN || errno == EWOULDBLOCK)
					return;
			} else {
				if (errno == EINTR)
					continue;
			}

			_exit(ERR_EXEC_RECVMSG);
		}

		receive_len += len;

		if (msg.msg_flags & MSG_CTRUNC)
			_exit(ERR_EXEC_MSG_CTRUNC);

		struct cmsghdr *cmsg = CMSG_FIRSTHDR(&msg);
		if (cmsg) {
			handle_control_message(sending, cmsg);

			// Only one message per sizeof(pid_t) bytes, otherwise
			// we may overflow sending buffer.
			if (CMSG_NXTHDR(&msg, cmsg))
				_exit(ERR_EXEC_CMSG_NXTHDR);
		}
	}

	if (receive.pid != 0)
		handle_pid_message(killed, died, receive.pid);
}

static inline void handle_sending(struct buffer *sending)
{
	ssize_t len = buffer_send(sending, GATE_CONTROL_FD, MSG_DONTWAIT);
	if (len <= 0) {
		if (len == 0)
			_exit(0);

		if (errno == EAGAIN || errno == EWOULDBLOCK)
			return;

		_exit(ERR_EXEC_SEND);
	}
}

static inline void handle_reaping(struct buffer *sending, struct buffer *killed, struct buffer *died)
{
	while (1) {
		int status;
		pid_t pid = waitpid(-1, &status, WNOHANG);
		if (pid <= 0) {
			if (pid == 0 || errno == ECHILD)
				return;

			_exit(ERR_EXEC_WAITPID);
		}

		if (WIFSTOPPED(status) || WIFCONTINUED(status))
			continue;

		const struct send_entry entry = {pid, status};
		if (buffer_append(sending, &entry, sizeof entry) != 0)
			_exit(ERR_EXEC_SENDBUF_OVERFLOW_REAP);

		if (!buffer_remove_pid(killed, pid))
			if (buffer_append_pid(died, pid) != 0)
				_exit(ERR_EXEC_DEADBUF_OVERFLOW);
	}
}

static void sandbox_common(void)
{
	if (prctl(PR_SET_DUMPABLE, 0) != 0)
		_exit(ERR_EXEC_PRCTL_NOT_DUMPABLE);

	xlimit(RLIMIT_DATA, GATE_LIMIT_DATA, ERR_EXEC_SETRLIMIT_DATA);

	// TODO: seccomp
}

int main(void)
{
	xcloexec(GATE_DEBUG_FD); // In case it is not set by child.
	xcloexec(GATE_CONTROL_FD);
	xcloexec(GATE_LOADER_FD);

	init_sigchld();

	char buffers[BUFFER_STORAGE_SIZE(SENDING_CAPACITY + KILLED_CAPACITY + DIED_CAPACITY)];
	struct buffer sending = BUFFER_INITIALIZER(buffers, 0);
	struct buffer killed = BUFFER_INITIALIZER(buffers, SENDING_CAPACITY);
	struct buffer died = BUFFER_INITIALIZER(buffers, SENDING_CAPACITY + KILLED_CAPACITY);

	if (GATE_SANDBOX)
		sandbox_common();

	while (1) {
		int revents = do_ppoll(&sending, &killed, &died);
		if (revents < 0) {
			handle_reaping(&sending, &killed, &died);
		} else {
			if (revents & POLLOUT)
				handle_sending(&sending);

			if (revents & POLLIN)
				handle_receiving(&sending, &killed, &died);
		}
	}
}
