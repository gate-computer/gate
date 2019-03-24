// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <gate.h>

int check(void)
{
	uint64_t value = gate_randomseed();

	gate_debug_hex(value);

	// If this happens, it's probably due to a bug; detect it.
	if ((value & 0xffffffff) == 0 || (value >> 32) == 0)
		return 1;

	// This is not a useful property, but if it doesn't hold, it's
	// indicative of a problem.
	for (int i = 0; i < 10; i++)
		if (gate_randomseed() != value)
			return 1;

	return 0;
}
