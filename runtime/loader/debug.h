// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#ifndef GATE_RUNTIME_LOADER_DEBUG_H
#define GATE_RUNTIME_LOADER_DEBUG_H

#include <stddef.h>
#include <stdint.h>

#include <sys/syscall.h>

#include "syscall.h"

#define debug_generic_func(x) _Generic((x), /* clang-format off */ \
		_Bool:                  debug_uint, \
		signed char:            debug_int,  \
		signed short int:       debug_int,  \
		signed int:             debug_int,  \
		signed long int:        debug_int,  \
		signed long long int:   debug_int,  \
		unsigned char:          debug_uint, \
		unsigned short int:     debug_uint, \
		unsigned int:           debug_uint, \
		unsigned long int:      debug_uint, \
		unsigned long long int: debug_uint, \
		const char *:           debug_str,  \
		char *:                 debug_str,  \
		const void *:           debug_ptr,  \
		void *:                 debug_ptr,  \
		default:                debug_type_not_supported \
	) /* clang-format on */

#define debug1(a) \
	do { \
		debug_generic_func(a)(a); \
	} while (0)

#define debug2(a, b) \
	do { \
		debug_generic_func(a)(a); \
		debug_generic_func(b)(b); \
	} while (0)

#define debug3(a, b, c) \
	do { \
		debug_generic_func(a)(a); \
		debug_generic_func(b)(b); \
		debug_generic_func(c)(c); \
	} while (0)

#define debug4(a, b, c, d) \
	do { \
		debug_generic_func(a)(a); \
		debug_generic_func(b)(b); \
		debug_generic_func(c)(c); \
		debug_generic_func(d)(d); \
	} while (0)

#define debug5(a, b, c, d, e) \
	do { \
		debug_generic_func(a)(a); \
		debug_generic_func(b)(b); \
		debug_generic_func(c)(c); \
		debug_generic_func(d)(d); \
		debug_generic_func(e)(e); \
	} while (0)

#define debug6(a, b, c, d, e, f) \
	do { \
		debug_generic_func(a)(a); \
		debug_generic_func(b)(b); \
		debug_generic_func(c)(c); \
		debug_generic_func(d)(d); \
		debug_generic_func(e)(e); \
		debug_generic_func(f)(f); \
	} while (0)

#define debug debug1

void debug_type_not_supported(void); // No implementation.

static inline void debug_data(const void *data, size_t size)
{
	syscall3(SYS_write, 2, (uintptr_t) data, size);
}

static inline void debug_uint(uint64_t n)
{
	char buf[20];
	int i = sizeof buf;

	do {
		buf[--i] = '0' + (n % 10);
		n /= 10;
	} while (n);

	debug_data(buf + i, sizeof buf - i);
}

static inline void debug_int(int64_t n)
{
	uint64_t u;

	if (n >= 0) {
		u = n;
	} else {
		const char sign[1] = {'-'};
		debug_data(sign, sizeof sign);

		u = ~n + 1;
	}

	debug_uint(u);
}

static inline void debug_hex(uint64_t n)
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

	debug_data(buf + i, sizeof buf - i);
}

static inline void debug_ptr(const void *ptr)
{
	debug_data("0x", 2);
	debug_hex((uintptr_t) ptr);
}

static inline void debug_str(const char *s)
{
	size_t size = 0;

	for (const char *ptr = s; *ptr != '\0'; ptr++)
		size++;

	debug_data(s, size);
}

#endif
