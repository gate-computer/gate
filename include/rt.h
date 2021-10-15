// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This is a public API for writing Gate runtime libraries.  (It is really only
// suitable for supporting tests.)

#include <stddef.h>
#include <stdint.h>

#ifndef RT_TRAP_ENUM
enum trap {
	TRAP_SUCCESS = 0,
	TRAP_FAILURE = 1,
};
#endif

// Write 8 bytes of packet data or terminate.
void rt_write8(uint64_t value);

// Read 8 bytes of packet data or terminate.
uint64_t rt_read8(void);

// Terminate with result 0 or 1.  Other values are undefined.
void rt_trap(enum trap status) __attribute__((noreturn));

// Write to debug log.
void rt_debug(const char *str, size_t len);
