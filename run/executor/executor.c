#define _GNU_SOURCE

#include <assert.h>
#include <errno.h>
#include <signal.h>
#include <stddef.h>
#include <stdlib.h>

#include <fcntl.h>
#include <poll.h>
#include <sys/prctl.h>
#include <sys/resource.h>
#include <sys/socket.h>
#include <sys/time.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <unistd.h>

#include "../defs.h"
#include "buffer.h"

#define CHILD_NICE 19

#define SENDING_CAPACITY (BUFFER_MAX_ENTRIES * sizeof (struct send_entry))
#define KILLED_CAPACITY  (BUFFER_MAX_ENTRIES * sizeof (pid_t))
#define DIED_CAPACITY    (BUFFER_MAX_ENTRIES * sizeof (pid_t))

// executor.go relies on this assumption
static_assert(sizeof (pid_t) == sizeof (int32_t), "pid_t size");

// send_entry is like recvEntry in executor.go
struct send_entry {
	pid_t pid;      // negated value acknowledges execution request
	int32_t status; // defined only if pid is positive
} __attribute__ ((packed));

// Set close-on-exec flag on a file descriptor or die.
static void xcloexec(int fd)
{
	int flags = fcntl(fd, F_GETFD);
	if (flags < 0)
		_exit(10);

	if (fcntl(fd, F_SETFD, flags|FD_CLOEXEC) < 0)
		_exit(11);
}

// Set a resource limit or die.
static void xlimit(int resource, rlim_t rlim)
{
	struct rlimit buf;

	buf.rlim_cur = rlim;
	buf.rlim_max = rlim;

	if (setrlimit(resource, &buf) != 0)
		_exit(12);
}

// Duplicate a file descriptor or die.
static void xdup2(int oldfd, int newfd)
{
	if (dup2(oldfd, newfd) != newfd)
		_exit(13);
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
		_exit(14);

	if (signal(SIGCHLD, sighandler) == SIG_ERR)
		_exit(15);
}

__attribute__ ((noreturn))
static inline void execute_child(const int *fds, int num_fds)
{
	// file descriptor duplication order is fragile

	int debugfd = STDOUT_FILENO; // /dev/null
	if (num_fds > 3)
		debugfd = fds[3];

	xdup2(debugfd, GATE_DEBUG_FD);
	xdup2(fds[0], GATE_BLOCK_FD);
	xdup2(fds[1], GATE_OUTPUT_FD);
	xdup2(fds[2], GATE_MAPS_FD);

	if (nice(CHILD_NICE) != CHILD_NICE)
		_exit(16);

	xlimit(RLIMIT_NOFILE, GATE_LIMIT_NOFILE);
	xlimit(RLIMIT_NPROC, 0);

	if (prctl(PR_SET_TSC, PR_TSC_SIGSEGV, 0, 0, 0) != 0)
		_exit(17);

	char *empty[] = {NULL};
	fexecve(GATE_LOADER_FD, empty, empty);
	_exit(18);
}

__attribute__ ((noinline))
static pid_t spawn_child(const int *fds, int num_fds)
{
	pid_t pid = vfork();
	if (pid == 0)
		execute_child(fds, num_fds);

	return pid;
}

// Set up polling and signal mask according to buffer status and return revents
// by value.
static inline int do_ppoll(struct buffer *sending, struct buffer *killed, struct buffer *died)
{
	struct pollfd pollfd = {
		.fd     = GATE_CONTROL_FD,
	};

	sigset_t pollmask_storage;
	sigset_t *pollmask = NULL;

	if (buffer_space(sending, sizeof (struct send_entry))) {
		if (buffer_space_pid(killed))
			pollfd.events |= POLLIN;

		if (buffer_space_pid(died)) {
			// enable reaping
			pollmask = &pollmask_storage;
			sigemptyset(pollmask);
		}
	}

	if (buffer_content(sending))
		pollfd.events |= POLLOUT;

	int count = ppoll(&pollfd, 1, NULL, pollmask); // may invoke sighandler
	if (count < 0) {
		if (errno == EINTR)
			return -1; // reap

		_exit(19);
	}

	return (unsigned short) pollfd.revents; // zero extension
}

static inline ssize_t do_recvmsg(struct msghdr *msg, void *buf, size_t buflen, int flags)
{
	struct iovec io = {
		.iov_base       = buf,
		.iov_len        = buflen,
	};

	msg->msg_iov = &io;
	msg->msg_iovlen = 1;

	return recvmsg(GATE_CONTROL_FD, msg, flags);
}

static inline void handle_control_message(struct buffer *sending, struct cmsghdr *cmsg)
{
	if (cmsg->cmsg_level != SOL_SOCKET)
		_exit(20);

	if (cmsg->cmsg_type != SCM_RIGHTS)
		_exit(21);

	int num_fds;
	if (cmsg->cmsg_len == CMSG_LEN(3 * sizeof (int)))
		num_fds = 3;
	else if (cmsg->cmsg_len == CMSG_LEN(4 * sizeof (int)))
		num_fds = 4;
	else
		_exit(22);

	const int *fds = (int *) CMSG_DATA(cmsg);

	pid_t pid = spawn_child(fds, num_fds);
	if (pid <= 0)
		_exit(23);

	for (int i = 0; i < num_fds; i++)
		close(fds[i]);

	const struct send_entry entry = {-pid, 0};
	if (buffer_append(sending, &entry, sizeof (entry)) != 0)
		_exit(24);
}

static inline void handle_pid_message(struct buffer *killed, struct buffer *died, pid_t pid)
{
	if (pid <= 1)
		_exit(25);

	if (!buffer_remove_pid(died, pid)) {
		if (kill(pid, SIGKILL) != 0)
			_exit(26);

		if (buffer_append_pid(killed, pid) != 0)
			_exit(27);
	}
}

static inline void handle_receiving(struct buffer *sending, struct buffer *killed, struct buffer *died)
{
	union {
		char buf[sizeof (pid_t)];
		pid_t pid;
	} receive;

	for (size_t receive_len = 0; receive_len < sizeof (receive.buf); ) {
		union {
			char buf[CMSG_SPACE(4 * sizeof (int))];
			struct cmsghdr alignment;
		} ctl;

		struct msghdr msg = {
			.msg_control    = ctl.buf,
			.msg_controllen = sizeof (ctl.buf),
		};

		int flags = MSG_CMSG_CLOEXEC;
		if (receive_len == 0)
			flags |= MSG_DONTWAIT;

		ssize_t len = do_recvmsg(&msg, receive.buf + receive_len, sizeof (receive.buf) - receive_len, flags);
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

			_exit(28);
		}

		receive_len += len;

		if (msg.msg_flags & MSG_CTRUNC)
			_exit(29);

		struct cmsghdr *cmsg = CMSG_FIRSTHDR(&msg);
		if (cmsg) {
			handle_control_message(sending, cmsg);

			// only one message per sizeof (pid_t) bytes, otherwise
			// we may overflow sending buffer
			if (CMSG_NXTHDR(&msg, cmsg))
				_exit(30);
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

		_exit(31);
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

			_exit(32);
		}

		if (WIFSTOPPED(status) || WIFCONTINUED(status))
			continue;

		const struct send_entry entry = {pid, status};
		if (buffer_append(sending, &entry, sizeof (entry)) != 0)
			_exit(33);

		if (!buffer_remove_pid(killed, pid))
			if (buffer_append_pid(died, pid) != 0)
				_exit(34);
	}
}

int main()
{
	xlimit(RLIMIT_DATA, 0);

	xcloexec(GATE_CONTROL_FD);
	xcloexec(GATE_LOADER_FD);

	init_sigchld();

	char buffers[BUFFER_STORAGE_SIZE(SENDING_CAPACITY + KILLED_CAPACITY + DIED_CAPACITY)];
	struct buffer sending = BUFFER_INITIALIZER(buffers, 0);
	struct buffer killed = BUFFER_INITIALIZER(buffers, SENDING_CAPACITY);
	struct buffer died = BUFFER_INITIALIZER(buffers, SENDING_CAPACITY + KILLED_CAPACITY);

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
