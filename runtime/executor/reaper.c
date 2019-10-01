// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#define _GNU_SOURCE

#include "reaper.h"

#include <errno.h>
#include <signal.h>
#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>
#include <string.h>
#include <time.h>

#include <sys/types.h>
#include <sys/uio.h>
#include <sys/wait.h>
#include <unistd.h>

#include "errors.h"
#include "executor.h"
#include "map.h"
#include "queue.h"
#include "runtime.h"

struct queue {
	struct exec_status buf[QUEUE_BUFLEN];
	unsigned int begin;
	unsigned int end;
	unsigned int transformed;
};

static void queue_init(struct queue *q)
{
	memset(q, 0, sizeof(struct queue));
}

static bool queue_empty(const struct queue *q)
{
	return q->begin == q->end;
}

static bool queue_full(const struct queue *q)
{
	return queue_wrap(q->end + 1) == q->begin;
}

static void queue_buffer(struct queue *q, pid_t pid, int32_t status)
{
	q->buf[q->end].pid = pid;
	q->buf[q->end].status = status;
	q->end = queue_wrap(q->end + 1);
}

static void queue_consume(struct queue *q, unsigned int count)
{
	q->begin = queue_wrap(q->begin + count);
}

static bool queue_transform_remove(struct queue *q, struct pid_map *map)
{
	q->transformed = pid_map_remove_transform(map, q->buf, q->transformed, q->end);
	return q->transformed == q->end;
}

static int kill_all(void)
{
	pid_t gid;
	int sig;

	if (GATE_SANDBOX && !no_namespaces) {
		// This process is pid 1 so it won't be killed.
		gid = 1;
		sig = SIGKILL;
	} else {
		// SIGTERM is blocked by this process but not by children.
		gid = getpgrp();
		sig = SIGTERM;
	}

	return kill(-gid, sig);
}

static void die(int code)
{
	kill_all();
	_exit(code);
}

static void signal_handler(int signum)
{
	kill_all();
	kill(getpid(), signum); // Handler was reset; simulate real termination reason.
	_exit(ERR_EXEC_RAISE);
}

static void configure_signal(sigset_t *runtime_mask, int signum)
{
	const struct sigaction action = {
		.sa_handler = signal_handler,
		.sa_flags = SA_RESETHAND,
	};

	if (sigaction(signum, &action, NULL) != 0)
		_exit(ERR_EXEC_SIGACTION);

	sigdelset(runtime_mask, signum); // Unblock signal.
}

NORETURN
void reaper(struct params *args)
{
	sigset_t sigmask;
	sigfillset(&sigmask); // Block all signals by default.

	configure_signal(&sigmask, SIGILL);
	configure_signal(&sigmask, SIGFPE);
	configure_signal(&sigmask, SIGSEGV);
	configure_signal(&sigmask, SIGBUS);

	if (pthread_sigmask(SIG_SETMASK, &sigmask, NULL) != 0)
		_exit(ERR_EXEC_SIGMASK);

	struct pid_map *map = &args->pid_map;
	pid_t sentinel_pid = args->sentinel_pid;

	struct queue q;
	queue_init(&q);

	while (1) {
		while (!queue_full(&q)) {
			int options = 0;
			if (!queue_empty(&q))
				options |= WNOHANG;

			int status;
			pid_t pid = waitpid(-1, &status, options);
			if (pid < 0) {
				if (errno == ECHILD && sentinel_pid == 0)
					die(0);

				die(ERR_REAP_WAITPID);
			}

			if (pid == 0)
				break;

			if (pid == sentinel_pid) {
				if (WIFSIGNALED(status) && WTERMSIG(status) == SIGTERM) {
					sentinel_pid = 0;
					continue;
				}

				die(ERR_REAP_SENTINEL);
			}

			queue_buffer(&q, pid, status);
		}

		if (!queue_transform_remove(&q, map)) {
			// A child process might die before executor has a
			// chance to insert its entry into map, but the
			// executor will do it right away when it is scheduled.

			if (queue_full(&q)) {
				struct timespec delay = {
					.tv_sec = 0,
					.tv_nsec = 1,
				};

				nanosleep(&delay, NULL);
			}

			continue;
		}

		struct iovec iov[2];
		int spans;

		if (q.end == 0) {
			iov[0].iov_base = &q.buf[q.begin];
			iov[0].iov_len = (QUEUE_BUFLEN - q.begin) * sizeof(q.buf[0]);
			spans = 1;
		} else if (q.begin < q.end) {
			iov[0].iov_base = &q.buf[q.begin];
			iov[0].iov_len = (q.end - q.begin) * sizeof(q.buf[0]);
			spans = 1;
		} else {
			iov[0].iov_base = &q.buf[q.begin];
			iov[0].iov_len = (QUEUE_BUFLEN - q.begin) * sizeof(q.buf[0]);
			iov[1].iov_base = &q.buf[0];
			iov[1].iov_len = q.end * sizeof(q.buf[0]);
			spans = 2;
		}

		ssize_t len = writev(GATE_CONTROL_FD, iov, spans);
		if (len <= 0) {
			if (len == 0)
				die(0);

			die(ERR_REAP_WRITEV);
		}

		if (len & (sizeof q.buf[0] - 1))
			die(ERR_REAP_WRITE_ALIGN);

		queue_consume(&q, len / sizeof q.buf[0]);
	}
}
