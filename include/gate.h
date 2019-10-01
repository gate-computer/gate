// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This is a low-level API for Gate user programs.  It is a thin wrapper on top
// of the Gate runtime ABI, with debug helpers.  It uses alternative ABI
// symbols in the "env" namespace due to nascent compiler support.

#ifndef _GATE_H
#define _GATE_H

#include <stddef.h>
#include <stdint.h>

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

#ifndef GATE_MAX_PACKET_SIZE
#define GATE_MAX_PACKET_SIZE 65536
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

#ifndef GATE_RESTRICT
#ifdef __cplusplus
#define GATE_RESTRICT __restrict__
#else
#define GATE_RESTRICT restrict
#endif
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
#define __GATE_IO __GATE_SYMVER(__gate_io, GATE_MAX_PACKET_SIZE)

void __gate_debug(const void *data, size_t len) GATE_NOEXCEPT;
void __gate_debug_type_not_supported(void); // No implementation.
GATE_NORETURN void __gate_exit(int status) GATE_NOEXCEPT;
void __GATE_IO(void *GATE_RESTRICT recv, size_t *GATE_RESTRICT recvlen, const void *GATE_RESTRICT send, size_t *GATE_RESTRICT sendlen, unsigned flags) GATE_NOEXCEPT;
uint64_t __gate_randomseed(void) GATE_NOEXCEPT;
int __gate_time(int clockid, int64_t buf[2]) GATE_NOEXCEPT;

static inline void __gate_debug_str(const char *s) GATE_NOEXCEPT
{
	size_t size = 0;

	for (const char *ptr = s; *ptr != '\0'; ptr++)
		size++;

	__gate_debug(s, size);
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

	__gate_debug(buf + i, sizeof buf - i);
}

static inline void __gate_debug_uint(uint64_t n) GATE_NOEXCEPT
{
	char buf[20];
	int i = sizeof buf;

	do {
		buf[--i] = '0' + (n % 10);
		n /= 10;
	} while (n);

	__gate_debug(buf + i, sizeof buf - i);
}

static inline void __gate_debug_int(int64_t n) GATE_NOEXCEPT
{
	uint64_t u;

	if (n >= 0) {
		u = n;
	} else {
		const char sign[1] = {'-'};
		__gate_debug(sign, sizeof sign);

		u = ~n + 1;
	}

	__gate_debug_uint(u);
}

// Public C API (excluding struct members starting with underscore)

#define GATE_API_VERSION 0

#define GATE_IO_RECV_WAIT 0x1

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

enum gate_clockid {
	GATE_CLOCK_REALTIME,
	GATE_CLOCK_MONOTONIC,
};

struct gate_timespec {
	int64_t sec;
	long nsec;
};

struct gate_packet {
	uint32_t size;
	int16_t code;
	uint8_t domain;
	uint8_t __reserved[1];
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
	__gate_debug("0x", 2);
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
	__gate_debug(data, size);
#endif
}

GATE_NORETURN
static inline void gate_exit(int status) GATE_NOEXCEPT
{
	__gate_exit(status);
}

static inline void gate_io(void *GATE_RESTRICT recv, size_t *GATE_RESTRICT recvlen, const void *GATE_RESTRICT send, size_t *GATE_RESTRICT sendlen, unsigned flags) GATE_NOEXCEPT
{
	__GATE_IO(recv, recvlen, send, sendlen, flags);
}

static inline uint64_t gate_randomseed(void) GATE_NOEXCEPT
{
	return __gate_randomseed();
}

static inline int gate_gettime(enum gate_clockid clk_id, struct gate_timespec *tp) GATE_NOEXCEPT
{
	int64_t buf[2];
	int ret = __gate_time(clk_id, buf);
	if (ret >= 0) {
		tp->sec = buf[0];
		tp->nsec = buf[1];
	}
	return ret;
}

#ifdef __cplusplus
}
#endif

#endif
