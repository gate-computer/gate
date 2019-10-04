// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <gate.h>

uint16_t __wasi_random_get(void *buf, size_t buflen);

void dump(void)
{
	uint64_t value[2];
	__wasi_random_get(value, sizeof value);

	gate_debug_hex(value[0]);
	gate_debug(" ");
	gate_debug_hex(value[1]);
}

void toomuch(void)
{
	gate_debug("ping");

	char value[17];
	__wasi_random_get(value, sizeof value);

	gate_debug("\nunreachable");
}

void toomuch2(void)
{
	char value[10];
	__wasi_random_get(value, sizeof value);

	gate_debug("ping");

	__wasi_random_get(value, sizeof value);

	gate_debug("\nunreachable");
}
