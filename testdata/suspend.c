// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <gate.h>

static void slow_nop(void)
{
	gate_io(NULL, NULL, NULL, NULL, 0);
}

static void delay(void)
{
	for (int i = 0; i < 10000000; i++)
		slow_nop();
}

static void iteration(long i)
{
	gate_debug2(i, "\n");
	delay();
}

int main(void)
{
	for (long i = 0;; i++)
		iteration(i);
}
