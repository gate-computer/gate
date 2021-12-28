// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <gate.h>

extern "C" {

uint16_t __wasi_clock_time_get(uint32_t, uint64_t, uint64_t*) noexcept;

int check()
{
	auto t = gate_clock_realtime();
	if (t < 1500000000000000000ULL)
		return 1;

	uint64_t t2;
	do {
		t2 = gate_clock_realtime();
	} while (t == t2);

	if (__wasi_clock_time_get(4, 1, &t) != 28) // EINVAL
		return 1;

	if (__wasi_clock_time_get(-1, 1, &t) != 28) // EINVAL
		return 1;

	return 0;
}

} // extern "C"
