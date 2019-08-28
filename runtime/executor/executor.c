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

#include "align.h"
#include "errors.h"
#include "executor.h"
#include "execveat.h"
#include "map.h"
#include "reaper.h"
#include "runtime.h"

#define NOINLINE __attribute__((noinline))

#define RECEIVE_BUFLEN 128

union control_buffer {
	char buf[CMSG_SPACE(2 * sizeof(int))]; // Space for 2 file descriptors.
	struct cmsghdr alignment;
};

// Close a file descriptor or die.
static void xclose(int fd)
{
	if (close(fd) != 0)
		_exit(ERR_SENTINEL_CLOSE);
}

static void sentinel_child(void)
{
	sigset_t sigmask;
	sigemptyset(&sigmask);
	if (pthread_sigmask(SIG_SETMASK, &sigmask, NULL) != 0)
		_exit(ERR_SENTINEL_SIGMASK);

	xclose(GATE_CONTROL_FD);
	xclose(GATE_LOADER_FD);

	if (prctl(PR_SET_PDEATHSIG, SIGKILL) != 0)
		_exit(ERR_SENTINEL_PRCTL_PDEATHSIG);

	pause();
	_exit(ERR_SENTINEL_PAUSE);
}

// Duplicate a file descriptor or die.
static void xdup2(int oldfd, int newfd)
{
	if (dup2(oldfd, newfd) != newfd)
		_exit(ERR_EXECHILD_DUP2);
}

NORETURN
static void execute_child(int input_fd, int output_fd)
{
	xdup2(input_fd, GATE_INPUT_FD);
	xdup2(output_fd, GATE_OUTPUT_FD);

	char *none[] = {NULL};

	sys_execveat(GATE_LOADER_FD, "", none, none, AT_EMPTY_PATH);
	_exit(ERR_EXECHILD_EXEC_LOADER);
}

NOINLINE
static pid_t spawn_child(int input_fd, int output_fd)
{
	pid_t pid = vfork();
	if (pid == 0)
		execute_child(input_fd, output_fd);

	return pid;
}

static pid_t create_process(struct cmsghdr *cmsg, struct pid_map *map, int16_t new_id, int16_t *old_id_out)
{
	if (cmsg->cmsg_level != SOL_SOCKET)
		_exit(ERR_EXEC_CMSG_LEVEL);

	if (cmsg->cmsg_type != SCM_RIGHTS)
		_exit(ERR_EXEC_CMSG_TYPE);

	if (cmsg->cmsg_len != CMSG_LEN(2 * sizeof(int)))
		_exit(ERR_EXEC_CMSG_LEN);

	const int *fds = (int *) CMSG_DATA(cmsg);

	pid_t pid = spawn_child(fds[0], fds[1]);
	if (pid <= 0)
		_exit(ERR_EXEC_VFORK);

	if (pid_map_replace(map, pid, new_id, old_id_out) < 0)
		_exit(ERR_EXEC_MAP_INSERT);

	close(fds[1]);
	close(fds[0]);

	return pid;
}

static bool signal_process(pid_t pid, int signum)
{
	if (pid == 0)
		return false;

	if (kill(pid, signum) != 0) {
		// The child might have been reaped after the map lookup.  No
		// processes are created between the map lookup and kill, so
		// the pid cannot have been reused.
		if (errno == ESRCH)
			return false;

		_exit(ERR_EXEC_KILL);
	}

	return true;
}

static void suspend_process(pid_t pid)
{
	if (!signal_process(pid, SIGXCPU))
		return;

	const struct rlimit cpu = {
		.rlim_cur = 1,
		.rlim_max = 1, // SIGKILL in one second.
	};

	if (prlimit(pid, RLIMIT_CPU, &cpu, NULL) != 0) {
		// See the comment in kill_existing.
		if (errno == ESRCH)
			return;

		_exit(ERR_EXEC_PRLIMIT_CPU);
	}
}

static void *executor(void *params)
{
	sigset_t sigmask;
	sigemptyset(&sigmask);
	sigaddset(&sigmask, SIGCHLD);
	if (pthread_sigmask(SIG_SETMASK, &sigmask, NULL) != 0)
		_exit(ERR_EXEC_SIGMASK);

	struct params *args = params;
	struct pid_map *map = &args->pid_map;
	pid_t sentinel_pid = args->sentinel_pid;
	pid_t *id_pids = args->id_pids;

	struct mmsghdr msgs[RECEIVE_BUFLEN];
	struct iovec iovs[RECEIVE_BUFLEN];
	struct exec_request reqs[RECEIVE_BUFLEN];
	union control_buffer ctls[RECEIVE_BUFLEN];

	memset(msgs, 0, sizeof msgs);
	memset(iovs, 0, sizeof iovs);

	for (int i = 0; i < RECEIVE_BUFLEN; i++) {
		iovs[i].iov_base = &reqs[i];
		iovs[i].iov_len = sizeof reqs[i];
		msgs[i].msg_hdr.msg_iov = &iovs[i];
		msgs[i].msg_hdr.msg_iovlen = 1;
		msgs[i].msg_hdr.msg_control = ctls[i].buf;
	}

	while (1) {
		for (int i = 0; i < RECEIVE_BUFLEN; i++)
			msgs[i].msg_hdr.msg_controllen = sizeof ctls[i].buf;

		int count = recvmmsg(GATE_CONTROL_FD, msgs, RECEIVE_BUFLEN, MSG_CMSG_CLOEXEC | MSG_WAITFORONE, NULL);
		if (count <= 0)
			_exit(ERR_EXEC_RECVMSG);

		for (int i = 0; i < count; i++) {
			if (msgs[i].msg_len == 0) {
				if (kill(sentinel_pid, SIGTERM) != 0)
					_exit(ERR_EXEC_KILL_SENTINEL);

				return NULL;
			}

			if (msgs[i].msg_len != sizeof reqs[i])
				_exit(ERR_EXEC_MSG_LEN);

			if (msgs[i].msg_hdr.msg_flags & MSG_CTRUNC)
				_exit(ERR_EXEC_MSG_CTRUNC);

			int16_t id = reqs[i].id;
			if (id < 0 || id >= ID_NUM)
				_exit(ERR_EXEC_ID_RANGE);

			struct cmsghdr *cmsg = CMSG_FIRSTHDR(&msgs[i].msg_hdr);
			int16_t old_id = -1;
			pid_t pid;

			switch (reqs[i].op) {
			case EXEC_OP_CREATE:
				if (cmsg == NULL)
					_exit(ERR_EXEC_CMSG_OP_MISMATCH);

				pid = create_process(cmsg, map, id, &old_id);
				if (old_id >= 0)
					id_pids[old_id] = 0;
				id_pids[id] = pid;

				// Only one control message per exec_request.
				if (CMSG_NXTHDR(&msgs[i].msg_hdr, cmsg))
					_exit(ERR_EXEC_CMSG_NXTHDR);
				break;

			case EXEC_OP_KILL:
				if (cmsg)
					_exit(ERR_EXEC_CMSG_OP_MISMATCH);

				signal_process(id_pids[id], SIGKILL);
				id_pids[id] = 0;
				break;

			case EXEC_OP_SUSPEND:
				if (cmsg)
					_exit(ERR_EXEC_CMSG_OP_MISMATCH);

				suspend_process(id_pids[id]);
				id_pids[id] = 0;
				break;

			default:
				_exit(ERR_EXEC_OP);
			}
		}
	}
}

// Set close-on-exec flag on a file descriptor or die.
static void set_cloexec(int fd)
{
	int flags = fcntl(fd, F_GETFD);
	if (flags < 0)
		_exit(ERR_EXEC_FCNTL_GETFD);

	if (fcntl(fd, F_SETFD, flags | FD_CLOEXEC) < 0)
		_exit(ERR_EXEC_FCNTL_CLOEXEC);
}

// Increase program break or die.
static void *xbrk(size_t size, long pagesize)
{
	size = align_size(size, pagesize);

	// musl doesn't support sbrk at all; use brk directly.
	unsigned long begin = syscall(SYS_brk, 0);
	unsigned long end = syscall(SYS_brk, begin + size);
	if (end != begin + size)
		_exit(ERR_EXEC_BRK);

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
		_exit(exitcode);
}

int main(void)
{
	set_cloexec(STDIN_FILENO);
	set_cloexec(STDOUT_FILENO);
	set_cloexec(STDERR_FILENO);
	set_cloexec(GATE_CONTROL_FD);
	set_cloexec(GATE_LOADER_FD);

	if (GATE_SANDBOX) {
		if (prctl(PR_SET_DUMPABLE, 0) != 0)
			_exit(ERR_EXEC_PRCTL_NOT_DUMPABLE);
	}

	// Block all signals during thread creation to avoid race conditions.
	sigset_t sigmask;
	sigfillset(&sigmask);
	sigdelset(&sigmask, SIGILL);
	sigdelset(&sigmask, SIGFPE);
	sigdelset(&sigmask, SIGSEGV);
	sigdelset(&sigmask, SIGBUS);
	if (pthread_sigmask(SIG_SETMASK, &sigmask, NULL) != 0)
		_exit(ERR_EXEC_SIGMASK);

	// Sentinel process ensures that waitpid doesn't fail with ECHILD
	// during normal operation.  Shutdown is signaled by its termination.
	pid_t sentinel_pid = fork();
	if (sentinel_pid < 0)
		_exit(ERR_EXEC_FORK_SENTINEL);
	if (sentinel_pid == 0)
		sentinel_child();

	long pagesize = sysconf(_SC_PAGESIZE);
	if (pagesize <= 0)
		_exit(ERR_EXEC_PAGESIZE);

	size_t stack_size = align_size(GATE_EXECUTOR_STACK_SIZE, pagesize);
	void *stack = xbrk(stack_size + sizeof(struct params), pagesize);
	struct params *args = stack + stack_size;
	pid_map_init(&args->pid_map);
	args->sentinel_pid = sentinel_pid;

	if (GATE_SANDBOX) {
		xsetrlimit(RLIMIT_DATA, GATE_LIMIT_DATA, ERR_EXEC_SETRLIMIT_DATA);
		xsetrlimit(RLIMIT_STACK, align_size(GATE_LOADER_STACK_SIZE, pagesize), ERR_EXEC_SETRLIMIT_STACK);
	}

	// ASLR makes stack size and stack pointer position unpredictable, so
	// it's hard to unmap the initial stack.  Run-time mapping addresses
	// are randomized manually.
	if (personality(ADDR_NO_RANDOMIZE) < 0)
		_exit(ERR_EXEC_PERSONALITY_ADDR_NO_RANDOMIZE);

	pthread_t thread;
	pthread_attr_t thread_attr;

	if (pthread_attr_init(&thread_attr) != 0)
		_exit(ERR_EXEC_THREAD_ATTR);

	if (pthread_attr_setdetachstate(&thread_attr, PTHREAD_CREATE_DETACHED) != 0)
		_exit(ERR_EXEC_THREAD_ATTR);

	if (pthread_attr_setstack(&thread_attr, stack, stack_size) != 0)
		_exit(ERR_EXEC_THREAD_ATTR);

	if (pthread_create(&thread, &thread_attr, executor, args) != 0)
		_exit(ERR_EXEC_THREAD_CREATE);

	if (pthread_attr_destroy(&thread_attr) != 0)
		_exit(ERR_EXEC_THREAD_ATTR);

	reaper(args);
}
