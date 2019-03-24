// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <gate.h>

int check(void)
{
	struct gate_timespec t = {0, -1};

	if (gate_gettime(GATE_CLOCK_REALTIME, &t) != 0)
		return 1;

	if (t.sec < 1500000000 || t.nsec < 0 || t.nsec >= 1000000000)
		return 1;

	struct gate_timespec t2;
	do {
		if (gate_gettime(GATE_CLOCK_REALTIME, &t2) != 0)
			return 1;
	} while (t.sec == t2.sec && t.nsec == t2.nsec);

	if (gate_gettime(2, &t) != -1 || gate_gettime(-1, &t) != -1)
		return 1;

	return 0;
}
