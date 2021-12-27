// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Update ../library.go by running 'go generate' in parent directory.

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#define EXPORT __attribute__((visibility("default")))

enum trap {
	TRAP_TERMINATE_SUCCESS = 2,
	TRAP_TERMINATE_FAILURE = 3,
	TRAP_ABI_DEFICIENCY = 127,
};

#define RT_TRAP_ENUM
#include <rt.h>

#define CLOCKFLAG_ABSTIME (1 << 0)

#define RETURN_FAULT_IF(condition)          \
	do {                                \
		if (condition)              \
			return ERROR_FAULT; \
	} while (0)

enum error {
	OK = 0,
	ERROR_AGAIN = 6,
	ERROR_BADF = 8,
	ERROR_FAULT = 21,
	ERROR_INVAL = 28,
	ERROR_NOENT = 44,
	ERROR_NOTSOCK = 57,
	ERROR_PERM = 63,
	ERROR_SPIPE = 70,
	ERROR_NOTCAPABLE = 76,
};

enum fd {
	FD_STDIN = 0,
	FD_STDOUT = 1,
	FD_STDERR = 2,
	FD_GATE = 4,
};

enum clock {
	CLOCK_REALTIME = 0,
	CLOCK_MONOTONIC = 1,
	CLOCKS = 4,
};

enum {
	EVENTTYPE_CLOCK = 0,
	EVENTTYPE_FD_READ = 1,
	EVENTTYPE_FD_WRITE = 2,
};

enum {
	FDFLAG_NONBLOCK = 0x4,
};

enum {
	RIGHT_FD_READ = 0x2,
	RIGHT_FD_WRITE = 0x40,
};

enum {
	FILETYPE_UNKNOWN = 0,
};

enum rt_clock {
	RT_CLOCK_REALTIME_COARSE = 5,
	RT_CLOCK_MONOTONIC_COARSE = 6,
};

enum rt_events {
	RT_POLLIN = 0x1,
	RT_POLLOUT = 0x4,
};

struct iovec {
	void *iov_base;
	uint32_t iov_len;
};

struct fdstat {
	uint8_t fs_filetype;
	uint16_t fs_flags;
	uint64_t fs_rights_base;
	uint64_t fs_rights_inheriting;
};

struct subscription {
	uint64_t userdata;
	uint8_t type;

	union {
		struct {
			uint32_t clockid;
			uint64_t timeout;
			uint64_t precision;
			uint16_t flags;
		} clock;

		struct {
			uint32_t fd;
		} fd_readwrite;
	} u;
};

struct event {
	uint64_t userdata;
	uint16_t error;
	uint8_t type;

	union {
		struct {
			uint64_t nbytes;
			uint16_t flags;
		} fd_readwrite;
	} u;
};

int rt_random(void);
uint64_t rt_time(enum rt_clock id);
enum rt_events rt_poll(enum rt_events in, enum rt_events out, int64_t nsec, int64_t sec); // Note order.
size_t rt_read(void *buf, size_t size);
size_t rt_write(const void *data, size_t size);

static inline uint64_t bytes64(uint8_t a0, uint8_t a1, uint8_t a2, uint8_t a3, uint8_t a4, uint8_t a5, uint8_t a6, uint8_t a7)
{
	return ((uint64_t)(a0) << 0x00) |
	       ((uint64_t)(a1) << 0x08) |
	       ((uint64_t)(a2) << 0x10) |
	       ((uint64_t)(a3) << 0x18) |
	       ((uint64_t)(a4) << 0x20) |
	       ((uint64_t)(a5) << 0x28) |
	       ((uint64_t)(a6) << 0x30) |
	       ((uint64_t)(a7) << 0x38);
}

static inline enum error fd_error(enum fd fd, enum error err)
{
	switch (fd) {
	case FD_STDIN:
	case FD_STDOUT:
	case FD_STDERR:
	case FD_GATE:
		return err;

	default:
		return ERROR_BADF;
	}
}

EXPORT
enum error args_get(char **argv, char *argvbuf)
{
	return OK;
}

EXPORT
enum error args_sizes_get(int32_t *argc_ptr, uint32_t *argvbufsize_ptr)
{
	RETURN_FAULT_IF(argc_ptr == NULL);
	RETURN_FAULT_IF(argvbufsize_ptr == NULL);

	*argc_ptr = 0;
	*argvbufsize_ptr = 0;
	return OK;
}

EXPORT
enum error clock_res_get(enum clock id, uint64_t *buf)
{
	if (id >= CLOCKS)
		return ERROR_INVAL;

	RETURN_FAULT_IF(buf == NULL);

	*buf = 1000000000ULL; // Worst-case scenario.
	return OK;
}

EXPORT
enum error clock_time_get(enum clock id, uint64_t precision, uint64_t *buf)
{
	enum rt_clock rt_id;

	if (id >= CLOCKS)
		return ERROR_INVAL;

	RETURN_FAULT_IF(buf == NULL);

	switch (id) {
	case CLOCK_REALTIME:
	case CLOCK_MONOTONIC:
		rt_id = id + RT_CLOCK_REALTIME_COARSE;
		break;

	default:
		rt_trap(TRAP_ABI_DEFICIENCY);
	}

	*buf = rt_time(rt_id);
	return OK;
}

EXPORT
enum error environ_get(void **env, uint64_t *buf)
{
	RETURN_FAULT_IF(env == NULL);
	RETURN_FAULT_IF(buf == NULL);

	buf[0] = bytes64('G', 'A', 'T', 'E', '_', 'A', 'B', 'I');
	buf[1] = bytes64('_', 'V', 'E', 'R', 'S', 'I', 'O', 'N');
	buf[2] = bytes64('=', '0', 0, 0, 0, 0, 0, 0);

	buf[3] = bytes64('G', 'A', 'T', 'E', '_', 'F', 'D', '=');
	buf[4] = bytes64('4', 0, 0, 0, 0, 0, 0, 0);

	buf[5] = bytes64('G', 'A', 'T', 'E', '_', 'M', 'A', 'X');
	buf[6] = bytes64('_', 'S', 'E', 'N', 'D', '_', 'S', 'I');
	buf[7] = bytes64('Z', 'E', '=', '6', '5', '5', '3', '6');
	buf[8] = bytes64(0, 0, 0, 0, 0, 0, 0, 0);

	env[0] = &buf[0];
	env[1] = &buf[3];
	env[2] = &buf[5];

	return OK;
}

EXPORT
enum error environ_sizes_get(int32_t *envlen_ptr, uint32_t *envbufsize_ptr)
{
	RETURN_FAULT_IF(envlen_ptr == NULL);
	RETURN_FAULT_IF(envbufsize_ptr == NULL);

	*envlen_ptr = 3;
	*envbufsize_ptr = 9 * sizeof(uint64_t);
	return OK;
}

EXPORT
enum fd fd(void)
{
	return FD_GATE;
}

EXPORT
enum error fd_close(enum fd fd)
{
	switch (fd) {
	case FD_STDIN:
	case FD_STDOUT:
	case FD_STDERR:
	case FD_GATE:
		rt_trap(TRAP_ABI_DEFICIENCY);

	default:
		return ERROR_BADF;
	}
}

EXPORT
enum error fd_fdstat_get(enum fd fd, struct fdstat *buf)
{
	RETURN_FAULT_IF(buf == NULL);

	uint16_t flags = 0;
	uint64_t rights = 0;

	switch (fd) {
	case FD_STDIN:
		break;

	case FD_STDOUT:
	case FD_STDERR:
		rights = RIGHT_FD_WRITE;
		break;

	case FD_GATE:
		flags = FDFLAG_NONBLOCK;
		rights = RIGHT_FD_READ | RIGHT_FD_WRITE;
		break;

	default:
		return ERROR_BADF;
	}

	buf->fs_filetype = FILETYPE_UNKNOWN;
	buf->fs_flags = flags;
	buf->fs_rights_base = rights;
	buf->fs_rights_inheriting = 0;
	return OK;
}

EXPORT
enum error fd_fdstat_set_rights(enum fd fd, uint64_t base, uint64_t inheriting)
{
	switch (fd) {
	case FD_STDIN:
		if (inheriting)
			return ERROR_NOTCAPABLE;
		if (base == 0)
			return OK;
		return ERROR_NOTCAPABLE;

	case FD_STDOUT:
	case FD_STDERR:
		if (inheriting)
			return ERROR_NOTCAPABLE;
		if (base == RIGHT_FD_WRITE)
			return OK;
		if (base)
			return ERROR_NOTCAPABLE;
		rt_trap(TRAP_ABI_DEFICIENCY);

	case FD_GATE:
		if (inheriting)
			return ERROR_NOTCAPABLE;
		if (base == (RIGHT_FD_READ | RIGHT_FD_WRITE))
			return OK;
		if (base & ~(uint64_t)(RIGHT_FD_READ | RIGHT_FD_WRITE))
			return ERROR_NOTCAPABLE;
		rt_trap(TRAP_ABI_DEFICIENCY);

	default:
		return ERROR_BADF;
	}
}

EXPORT
enum error fd_prestat_dir_name(enum fd fd, char *buf, size_t bufsize)
{
	RETURN_FAULT_IF(bufsize > 0 && buf == NULL);

	return fd_error(fd, ERROR_INVAL);
}

EXPORT
enum error fd_read(enum fd fd, const struct iovec *iov, int iovlen, uint32_t *nread_ptr)
{
	RETURN_FAULT_IF(iovlen > 0 && iov == NULL);
	RETURN_FAULT_IF(nread_ptr == NULL);

	size_t total = 0;

	switch (fd) {
	case FD_STDIN:
	case FD_STDOUT:
	case FD_STDERR:
		return ERROR_PERM;

	case FD_GATE:
		for (int i = 0; i < iovlen; i++) {
			size_t len = iov[i].iov_len;
			size_t n = rt_read(iov[i].iov_base, len);
			total += n;
			if (n < len) {
				if (total == 0)
					return ERROR_AGAIN;
				break;
			}
		}
		break;

	default:
		return ERROR_BADF;
	}

	*nread_ptr = total;
	return OK;
}

EXPORT
enum error fd_renumber(enum fd from, enum fd to)
{
	switch (from) {
	case FD_STDIN:
	case FD_STDOUT:
	case FD_STDERR:
	case FD_GATE:
		switch (to) {
		case FD_STDIN:
		case FD_STDOUT:
		case FD_STDERR:
		case FD_GATE:
			if (from == to)
				return OK;
			rt_trap(TRAP_ABI_DEFICIENCY);
		}
	}

	return ERROR_BADF;
}

EXPORT
enum error fd_write(enum fd fd, const struct iovec *iov, int iovlen, uint32_t *nwritten_ptr)
{
	RETURN_FAULT_IF(iovlen > 0 && iov == NULL);
	RETURN_FAULT_IF(nwritten_ptr == NULL);

	size_t total = 0;

	switch (fd) {
	case FD_STDIN:
		return ERROR_PERM;

	case FD_STDOUT:
	case FD_STDERR:
		for (int i = 0; i < iovlen; i++) {
			size_t len = iov[i].iov_len;
			rt_debug(iov[i].iov_base, len);
			total += len;
		}
		break;

	case FD_GATE:
		for (int i = 0; i < iovlen; i++) {
			size_t len = iov[i].iov_len;
			size_t n = rt_write(iov[i].iov_base, len);
			total += n;
			if (n < len) {
				if (total == 0)
					return ERROR_AGAIN;
				break;
			}
		}
		break;

	default:
		return ERROR_BADF;
	}

	*nwritten_ptr = total;
	return OK;
}

EXPORT
void io(const struct iovec *recv, int recvlen, uint32_t *nrecv_ptr, const struct iovec *send, int sendlen, uint32_t *nsent_ptr, int64_t timeout)
{
	enum rt_events events = RT_POLLIN | RT_POLLOUT;

	bool sending = false;
	for (int i = 0; i < sendlen; i++) {
		if (send[i].iov_len > 0) {
			sending = true;
			break;
		}
	}

	// Don't bother with sub-microsecond wait, unless it's the only task.
	if (timeout >= 0 && timeout < 1000) {
		if (sending)
			goto no_wait;

		for (int i = 0; i < recvlen; i++) {
			if (recv[i].iov_len > 0)
				goto no_wait;
		}
	}

	int64_t sec = -1;
	int64_t nsec = 0;
	if (timeout >= 0) {
		sec = timeout / 1000000000LL;
		nsec = timeout % 1000000000LL;
	}

	events = rt_poll(RT_POLLIN, sending ? RT_POLLOUT : 0, nsec, sec);

no_wait:;
	size_t nsent = 0;
	size_t nrecv = 0;

	if (events & RT_POLLOUT) {
		for (int i = 0; i < sendlen; i++) {
			size_t len = send[i].iov_len;
			size_t n = rt_write(send[i].iov_base, len);
			nsent += n;
			if (n < len)
				break;
		}
	}

	if (events & RT_POLLIN) {
		for (int i = 0; i < recvlen; i++) {
			size_t len = recv[i].iov_len;
			size_t n = rt_read(recv[i].iov_base, len);
			nrecv += n;
			if (n < len)
				break;
		}
	}

	if (nsent_ptr)
		*nsent_ptr = nsent;
	if (nrecv_ptr)
		*nrecv_ptr = nrecv;
}

// Avoid stack memory access and globals by not using array indexing.

struct timestamps {
	uint64_t realtime;
	uint64_t monotonic;
};

#define TIMESTAMP(container, id) \
	((id) == CLOCK_REALTIME ? container.realtime : container.monotonic)

#define SET_TIMESTAMP(container, id, value)      \
	do {                                     \
		uint64_t v = (value);            \
		if ((id) == CLOCK_REALTIME)      \
			container.realtime = v;  \
		else                             \
			container.monotonic = v; \
	} while (0)

EXPORT
enum error poll_oneoff(const struct subscription *sub, struct event *out, int nsub, uint32_t *nout_ptr)
{
	RETURN_FAULT_IF(nsub > 0 && sub == NULL);
	RETURN_FAULT_IF(nsub > 0 && out == NULL);
	RETURN_FAULT_IF(nout_ptr == NULL);

	enum rt_events pollin = 0;
	enum rt_events pollout = 0;
	bool have_timeout = false;
	uint64_t timeout = ~0ULL;
	struct timestamps begin = {0};

	for (int i = 0; i < nsub; i++) {
		uint32_t id;

		switch (sub[i].type) {
		case EVENTTYPE_CLOCK:
			id = sub[i].u.clock.clockid;
			if (id < CLOCKS) {
				if (id > CLOCK_MONOTONIC)
					rt_trap(TRAP_ABI_DEFICIENCY);

				enum rt_clock rt_id = id + RT_CLOCK_REALTIME_COARSE;

				uint64_t t = sub[i].u.clock.timeout;

				if (t != 0) { // Optimize special case.
					if (TIMESTAMP(begin, id) == 0)
						SET_TIMESTAMP(begin, id, rt_time(rt_id));
				}

				if (sub[i].u.clock.flags & CLOCKFLAG_ABSTIME) {
					uint64_t now = TIMESTAMP(begin, id);
					if (t < now)
						t = now;
					t -= now;
				}

				if (t < timeout)
					timeout = t;
				have_timeout = true;
				continue;
			}
			break;

		case EVENTTYPE_FD_READ:
			if (sub[i].u.fd_readwrite.fd == FD_GATE) {
				pollin = RT_POLLIN;
				continue;
			}
			break;

		case EVENTTYPE_FD_WRITE:
			if (sub[i].u.fd_readwrite.fd == FD_GATE) {
				pollout = RT_POLLOUT;
				continue;
			}
			break;
		}

		timeout = 0;
		have_timeout = true;
	}

	int64_t sec = -1;
	int64_t nsec = 0;
	if (have_timeout) {
		sec = timeout / 1000000000ULL;
		nsec = timeout % 1000000000ULL;
	}

	enum rt_events r = rt_poll(pollin, pollout, nsec, sec);

	struct timestamps end = {0};
	for (enum clock id = CLOCK_REALTIME; id <= CLOCK_MONOTONIC; id++) {
		if (TIMESTAMP(begin, id) != 0)
			SET_TIMESTAMP(end, id, rt_time(id + RT_CLOCK_REALTIME_COARSE));
	}

	int n = 0;

	for (int i = 0; i < nsub; i++) {
		uint32_t id;

		out[n].userdata = sub[i].userdata;
		out[n].error = 0;
		out[n].type = sub[i].type;
		out[n].u.fd_readwrite.nbytes = 0;
		out[n].u.fd_readwrite.flags = 0;

		switch (sub[i].type) {
		case EVENTTYPE_CLOCK:
			id = sub[i].u.clock.clockid;
			if (id < CLOCKS) {
				uint64_t t = sub[i].u.clock.timeout;

				if (t == 0) { // Optimize special case.
					n++;
					continue;
				}

				if ((sub[i].u.clock.flags & CLOCKFLAG_ABSTIME) == 0) {
					uint64_t abstime = TIMESTAMP(end, id) + t;
					if (abstime < t) // Overflow?
						continue;
					t = abstime;
				}

				if (t <= TIMESTAMP(end, id))
					n++;
				continue;
			}
			break;

		case EVENTTYPE_FD_READ:
			switch (sub[i].u.fd_readwrite.fd) {
			case FD_STDIN:
			case FD_STDOUT:
			case FD_STDERR:
				out[n].error = ERROR_PERM;
				n++;
				continue;

			case FD_GATE:
				if (r & RT_POLLIN) {
					out[n].u.fd_readwrite.nbytes = 65536;
					n++;
				}
				continue;

			default:
				out[n].error = ERROR_BADF;
				n++;
				continue;
			}
			break;

		case EVENTTYPE_FD_WRITE:
			switch (sub[i].u.fd_readwrite.fd) {
			case FD_STDIN:
				out[n].error = ERROR_PERM;
				n++;
				continue;

			case FD_STDOUT:
			case FD_STDERR:
				out[n].u.fd_readwrite.nbytes = 0x7fffffff;
				n++;
				continue;

			case FD_GATE:
				if (r & RT_POLLOUT) {
					out[n].u.fd_readwrite.nbytes = 65536;
					n++;
				}
				continue;

			default:
				out[n].error = ERROR_BADF;
				n++;
				continue;
			}
			break;
		}

		out[n].error = ERROR_INVAL;
		n++;
	}

	*nout_ptr = n;
	return OK;
}

EXPORT
void proc_exit(int status)
{
	rt_trap(status ? TRAP_TERMINATE_FAILURE : TRAP_TERMINATE_SUCCESS);
}

EXPORT
enum error proc_raise(int signal)
{
	rt_trap(TRAP_ABI_DEFICIENCY);
}

EXPORT
enum error random_get(uint8_t *buf, size_t len)
{
	RETURN_FAULT_IF(len > 0 && buf == NULL);

	while (len > 0) {
		int value = rt_random();
		if (value >= 0) {
			*buf++ = (uint8_t) value;
			len--;
		} else {
			rt_trap(TRAP_ABI_DEFICIENCY);
		}
	}

	return OK;
}

EXPORT
enum error sched_yield(void)
{
	return OK;
}

EXPORT
enum error sock_recv(enum fd fd, int a1, int a2, int a3, int a4, int a5)
{
	switch (fd) {
	case FD_STDIN:
	case FD_STDOUT:
	case FD_STDERR:
		return ERROR_PERM;

	case FD_GATE:
		return ERROR_NOTSOCK;

	default:
		return ERROR_BADF;
	}
}

EXPORT
enum error sock_send(enum fd fd, int a1, int a2, int a3, int a4)
{
	switch (fd) {
	case FD_STDIN:
		return ERROR_PERM;

	case FD_STDOUT:
	case FD_STDERR:
	case FD_GATE:
		return ERROR_NOTSOCK;

	default:
		return ERROR_BADF;
	}
}

EXPORT
enum error stub_fd(enum fd fd)
{
	return fd_error(fd, ERROR_PERM);
}

EXPORT
enum error stub_fd_i32(enum fd fd, int a1)
{
	return fd_error(fd, ERROR_PERM);
}

EXPORT
enum error stub_fd_i64(enum fd fd, int64_t a1)
{
	return fd_error(fd, ERROR_PERM);
}

EXPORT
enum error stub_fd_i32_i32(enum fd fd, int a1, int a2)
{
	return fd_error(fd, ERROR_PERM);
}

EXPORT
enum error stub_fd_i64_i64(enum fd fd, int64_t a1, int64_t a2)
{
	return fd_error(fd, ERROR_PERM);
}

EXPORT
enum error stub_fd_i64_i32_i32(enum fd fd, int64_t a1, int a2, int a3)
{
	return fd_error(fd, ERROR_PERM);
}

EXPORT
enum error stub_fd_i64_i64_i32(enum fd fd, int64_t a1, int64_t a2, int a3)
{
	return fd_error(fd, ERROR_PERM);
}

EXPORT
enum error stub_fd_i32_i32_i32_i32(enum fd fd, int a1, int a2, int a3, int a4)
{
	return fd_error(fd, ERROR_PERM);
}

EXPORT
enum error stub_i32_i32_fd_i32_i32(int a0, int a1, enum fd fd, int a3, int a4)
{
	return fd_error(fd, ERROR_PERM);
}

EXPORT
enum error stub_fd_i32_i32_i64_i32(enum fd fd, int a1, int a2, int64_t a3, int a4)
{
	return fd_error(fd, ERROR_PERM);
}

EXPORT
enum error stub_fd_i32_i32_fd_i32_i32(enum fd fd, int a1, int a2, enum fd fd3, int a4, int a5)
{
	return fd_error(fd, fd_error(fd3, ERROR_PERM));
}

EXPORT
enum error stub_fd_i32_i32_i32_i32_i32(enum fd fd, int a1, int a2, int a3, int a4, int a5)
{
	return fd_error(fd, ERROR_PERM);
}

EXPORT
enum error stub_fd_i32_i32_i32_fd_i32_i32(enum fd fd, int a1, int a2, int a3, enum fd fd4, int a5, int a6)
{
	return fd_error(fd, fd_error(fd4, ERROR_PERM));
}

EXPORT
enum error stub_fd_i32_i32_i32_i64_i64_i32(enum fd fd, int a1, int a2, int a3, int64_t a4, int64_t a5, int a6)
{
	return fd_error(fd, ERROR_PERM);
}

EXPORT
enum error stub_fd_i32_i32_i32_i32_i64_i64_i32_i32(enum fd fd, int a1, int a2, int a3, int a4, int64_t a5, int64_t a6, int a7, int a8)
{
	return fd_error(fd, ERROR_PERM);
}
