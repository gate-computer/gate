// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <gate.h>

int *__errno_location(void)
{
	static int errno_storage;
	return &errno_storage;
}

void abort(void)
{
	__gate_debug_write("\nAborted\n", 9);
	__gate_exit(1);
}

GATE_NORETURN
void exit(int status)
{
	__gate_exit(status);
}
