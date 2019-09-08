// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#ifndef GATE_RUNTIME_CAPS_H
#define GATE_RUNTIME_CAPS_H

#include <stdint.h>

#include <sys/syscall.h>

static inline int clear_caps(void)
{
	struct {
		uint32_t version;
		int pid;
	} header = {
		.version = 0x20080522, // ABI version 3.
		.pid = 0,
	};

	const struct {
		uint32_t effective, permitted, inheritable;
	} data[2] = {{0}, {0}};

	return syscall(SYS_capset, &header, data);
}

#endif
