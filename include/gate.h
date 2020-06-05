// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This is a low-level API for Gate user programs.  It is a thin wrapper on top
// of the Gate runtime ABI and WASI, with debug helpers.  It may use
// alternative ABI symbols in the "env" namespace.

#ifndef _GATE_H
#define _GATE_H

#include <stddef.h>
#include <stdint.h>

#ifdef __wasi__
#include <wasi/core.h>
#endif

#ifdef __cplusplus
extern "C" {
#endif

// C API configuration

#ifdef GATE_ABI_VERSION
#if GATE_ABI_VERSION != 0
#error GATE_ABI_VERSION not supported
#endif
#else
#define GATE_ABI_VERSION 0
#endif

#ifndef GATE_MAX_RECV_SIZE
#define GATE_MAX_RECV_SIZE 65536
#endif

#ifndef GATE_NOEXCEPT
#ifdef __cplusplus
#define GATE_NOEXCEPT noexcept
#else
#define GATE_NOEXCEPT
#endif
#endif

#ifndef GATE_NORETURN
#define GATE_NORETURN __attribute__((noreturn))
#endif

#ifndef GATE_PACKED
#define GATE_PACKED __attribute__((packed))
#endif

#ifndef GATE_PURE
#define GATE_PURE __attribute__((pure))
#endif

// Internal functions (not part of supported C API)

#ifdef __cplusplus
#define __GATE_DEBUG_BOOL_TYPE bool
#else
#define __GATE_DEBUG_BOOL_TYPE _Bool
#endif

#define __gate_debug_generic_func(x) _Generic((x), /* clang-format off */ \
		__GATE_DEBUG_BOOL_TYPE: gate_debug_uint, \
		signed char:            gate_debug_int,  \
		signed short int:       gate_debug_int,  \
		signed int:             gate_debug_int,  \
		signed long int:        gate_debug_int,  \
		signed long long int:   gate_debug_int,  \
		unsigned char:          gate_debug_uint, \
		unsigned short int:     gate_debug_uint, \
		unsigned int:           gate_debug_uint, \
		unsigned long int:      gate_debug_uint, \
		unsigned long long int: gate_debug_uint, \
		const char *:           gate_debug_str,  \
		char *:                 gate_debug_str,  \
		const void *:           gate_debug_ptr,  \
		void *:                 gate_debug_ptr,  \
		default:                __gate_debug_type_not_supported \
	) /* clang-format on */

#define __GATE_SYMVER_HELPER(name, num) name##_##num
#define __GATE_SYMVER(name, num) __GATE_SYMVER_HELPER(name, num)
#define __GATE_FD __GATE_SYMVER(__gate_fd, GATE_MAX_RECV_SIZE)
#define __GATE_IO __GATE_SYMVER(__gate_io, GATE_MAX_RECV_SIZE)

struct gate_iovec {
	void *iov_base;
	size_t iov_len;
};

void __gate_debug_type_not_supported(void); // No implementation.
GATE_PURE uint32_t __GATE_FD(void) GATE_NOEXCEPT;
void __GATE_IO(const struct gate_iovec *recv, int recvlen, size_t *recvsize, const struct gate_iovec *send, int sendlen, size_t *sendsize, unsigned flags) GATE_NOEXCEPT;

static inline void __gate_debug_data(const char *data, size_t size) GATE_NOEXCEPT
{
#ifdef __wasi__
	__wasi_ciovec_t iov = {(void *) data, size};
#else
	uint16_t __wasi_fd_write(uint32_t, const struct gate_iovec *, size_t, size_t *) GATE_NOEXCEPT;
	struct gate_iovec iov = {(void *) data, size};
#endif

	size_t written;
	(void) __wasi_fd_write(2, &iov, 1, &written);
}

static inline void __gate_debug_str(const char *s) GATE_NOEXCEPT
{
	size_t size = 0;

	for (const char *ptr = s; *ptr != '\0'; ptr++)
		size++;

	__gate_debug_data(s, size);
}

static inline void __gate_debug_hex(uint64_t n) GATE_NOEXCEPT
{
	char buf[16];
	int i = sizeof buf;

	do {
		int m = n & 15;
		char c;
		if (m < 10)
			c = '0' + m;
		else
			c = 'a' + (m - 10);
		buf[--i] = c;
		n >>= 4;
	} while (n);

	__gate_debug_data(buf + i, sizeof buf - i);
}

static inline void __gate_debug_uint(uint64_t n) GATE_NOEXCEPT
{
	char buf[20];
	int i = sizeof buf;

	do {
		buf[--i] = '0' + (n % 10);
		n /= 10;
	} while (n);

	__gate_debug_data(buf + i, sizeof buf - i);
}

static inline void __gate_debug_int(int64_t n) GATE_NOEXCEPT
{
	uint64_t u;

	if (n >= 0) {
		u = n;
	} else {
		const char sign[1] = {'-'};
		__gate_debug_data(sign, sizeof sign);

		u = ~n + 1;
	}

	__gate_debug_uint(u);
}

// Public C API (excluding struct members starting with underscore)

#define GATE_API_VERSION 0

#define GATE_IO_WAIT 0x1

#define GATE_PACKET_ALIGNMENT 8

#define GATE_PACKET_CODE_SERVICES -1

enum {
	GATE_PACKET_DOMAIN_CALL,
	GATE_PACKET_DOMAIN_INFO,
	GATE_PACKET_DOMAIN_FLOW,
	GATE_PACKET_DOMAIN_DATA,
};

#define GATE_SERVICE_STATE_AVAIL 0x1

#define GATE_ALIGN_PACKET(size) \
	(((size) + (size_t)(GATE_PACKET_ALIGNMENT - 1)) & ~(size_t)(GATE_PACKET_ALIGNMENT - 1))

#define gate_debug1(a)                           \
	do {                                     \
		__gate_debug_generic_func(a)(a); \
	} while (0)

#define gate_debug2(a, b)                        \
	do {                                     \
		__gate_debug_generic_func(a)(a); \
		__gate_debug_generic_func(b)(b); \
	} while (0)

#define gate_debug3(a, b, c)                     \
	do {                                     \
		__gate_debug_generic_func(a)(a); \
		__gate_debug_generic_func(b)(b); \
		__gate_debug_generic_func(c)(c); \
	} while (0)

#define gate_debug4(a, b, c, d)                  \
	do {                                     \
		__gate_debug_generic_func(a)(a); \
		__gate_debug_generic_func(b)(b); \
		__gate_debug_generic_func(c)(c); \
		__gate_debug_generic_func(d)(d); \
	} while (0)

#define gate_debug5(a, b, c, d, e)               \
	do {                                     \
		__gate_debug_generic_func(a)(a); \
		__gate_debug_generic_func(b)(b); \
		__gate_debug_generic_func(c)(c); \
		__gate_debug_generic_func(d)(d); \
		__gate_debug_generic_func(e)(e); \
	} while (0)

#define gate_debug6(a, b, c, d, e, f)            \
	do {                                     \
		__gate_debug_generic_func(a)(a); \
		__gate_debug_generic_func(b)(b); \
		__gate_debug_generic_func(c)(c); \
		__gate_debug_generic_func(d)(d); \
		__gate_debug_generic_func(e)(e); \
		__gate_debug_generic_func(f)(f); \
	} while (0)

#define gate_debug gate_debug1

struct gate_packet {
	uint32_t size;
	int16_t code;
	uint8_t domain;
	uint8_t index;
} GATE_PACKED;

struct gate_service_name_packet {
	struct gate_packet header;
	uint16_t count;
	char names[0]; // Variable length.
} GATE_PACKED;

struct gate_service_state_packet {
	struct gate_packet header;
	uint16_t count;
	uint8_t states[0]; // Variable length.
} GATE_PACKED;

struct gate_flow {
	int32_t id;
	int32_t increment;
} GATE_PACKED;

struct gate_flow_packet {
	struct gate_packet header;
	struct gate_flow flows[0]; // Variable length.
} GATE_PACKED;

struct gate_data_packet {
	struct gate_packet header;
	int32_t id;
	int32_t note;
	char data[0]; // Variable length.
} GATE_PACKED;

static inline uint64_t gate_clock_realtime(void) GATE_NOEXCEPT
{
#ifndef __wasi__
	uint16_t __wasi_clock_time_get(uint32_t, uint64_t, uint64_t *) GATE_NOEXCEPT;
#endif

	uint64_t t;
	(void) __wasi_clock_time_get(0, 1, &t);
	return t;
}

static inline uint64_t gate_clock_monotonic(void) GATE_NOEXCEPT
{
#ifndef __wasi__
	uint16_t __wasi_clock_time_get(uint32_t, uint64_t, uint64_t *) GATE_NOEXCEPT;
#endif

	uint64_t t;
	(void) __wasi_clock_time_get(1, 1, &t);
	return t;
}

static inline void gate_debug_int(int64_t n) GATE_NOEXCEPT
{
#ifdef NDEBUG
	(void) n;
#else
	__gate_debug_int(n);
#endif
}

static inline void gate_debug_uint(uint64_t n) GATE_NOEXCEPT
{
#ifdef NDEBUG
	(void) n;
#else
	__gate_debug_uint(n);
#endif
}

static inline void gate_debug_hex(uint64_t n) GATE_NOEXCEPT
{
#ifdef NDEBUG
	(void) n;
#else
	__gate_debug_hex(n);
#endif
}

static inline void gate_debug_ptr(const void *ptr) GATE_NOEXCEPT
{
#ifdef NDEBUG
	(void) ptr;
#else
	__gate_debug_data("0x", 2);
	__gate_debug_hex((uintptr_t) ptr);
#endif
}

static inline void gate_debug_str(const char *s) GATE_NOEXCEPT
{
#ifdef NDEBUG
	(void) s;
#else
	__gate_debug_str(s);
#endif
}

static inline void gate_debug_data(const char *data, size_t size) GATE_NOEXCEPT
{
#ifdef NDEBUG
	(void) data;
	(void) size;
#else
	__gate_debug_data(data, size);
#endif
}

GATE_NORETURN
static inline void gate_exit(int status) GATE_NOEXCEPT
{
#ifndef __wasi__
	GATE_NORETURN void __wasi_proc_exit(uint32_t) GATE_NOEXCEPT;
#endif

	__wasi_proc_exit(status);
}

static inline void gate_io(const struct gate_iovec *recv, int recvveclen, size_t *nreceived, const struct gate_iovec *send, int sendveclen, size_t *nsent, unsigned flags) GATE_NOEXCEPT
{
	__GATE_IO(recv, recvveclen, nreceived, send, sendveclen, nsent, flags);
}

static inline size_t gate_recv(void *buf, size_t size, unsigned flags) GATE_NOEXCEPT
{
	const struct gate_iovec iov = {buf, size};
	size_t n;
	__GATE_IO(&iov, 1, &n, NULL, 0, NULL, flags);
	return n;
}

static inline size_t gate_send(const void *data, size_t size) GATE_NOEXCEPT
{
	const struct gate_iovec iov = {(void *) data, size};
	size_t n;
	__GATE_IO(NULL, 0, NULL, &iov, 1, &n, 0);
	return n;
}

#ifdef __cplusplus
}
#endif

#endif
