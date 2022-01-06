// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#pragma once

#include <cstddef>
#include <cstdint>

#include <sys/syscall.h>

#include "syscall.hpp"

static inline void debug_data(void const* data, size_t size)
{
	syscall3(SYS_write, 2, uintptr_t(data), size);
}

static inline void debug(uint64_t n)
{
	char buf[20];
	int i = sizeof buf;

	do {
		buf[--i] = '0' + (n % 10);
		n /= 10;
	} while (n);

	debug_data(buf + i, sizeof buf - i);
}

static inline void debug(int64_t n)
{
	uint64_t u;

	if (n >= 0) {
		u = n;
	} else {
		const char sign[1] = {'-'};
		debug_data(sign, sizeof sign);

		u = ~n + 1;
	}

	debug(u);
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

static inline void debug(void const* ptr)
{
	debug_data("0x", 2);
	debug_hex(uintptr_t(ptr));
}

static inline void debug(char const* s)
{
	size_t size = 0;

	for (char const* ptr = s; *ptr != '\0'; ptr++)
		size++;

	debug_data(s, size);
}

static inline void debugln()
{
	debug("\n");
}

template <typename First, typename... Others>
static inline void debugln(First first, Others... others)
{
	debug(first);
	debugln(others...);
}
