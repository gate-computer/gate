// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <gate.h>

void *memcpy(void *dest, const void *src, size_t n)
{
	for (size_t i = 0; i < n; i++)
		((char *) dest)[i] = ((const char *) src)[i];
	return dest;
}

void *memset(void *s, int c, size_t n)
{
	for (size_t i = 0; i < n; i++)
		((char *) s)[i] = c;
	return s;
}

static void slow_nop(void)
{
	gate_io(NULL, 0, NULL, NULL, 0, NULL, 0);
}

static void delay(void)
{
	for (int i = 0; i < 1000000; i++)
		slow_nop();
}

static void iteration(long i)
{
	gate_debug3("suspend.c running: ", i, "\n");
	delay();
}

int loop(void)
{
	struct {
		struct gate_service_name_packet header;
		char names[25];
	} packet = {
		.header = {
			.header = {
				.size = sizeof packet,
				.code = GATE_PACKET_CODE_SERVICES,
			},
			.count = 3,
		},
		.names = "origin\0test\0_nonexistent",
	};

	// Send some uninitialized bytes from stack as padding.
	gate_send(&packet, GATE_ALIGN_PACKET(sizeof packet));

	for (long i = 0;; i++)
		iteration(i);
}

volatile unsigned long saved_mem = 0;

__attribute__((noinline)) unsigned long barrier(unsigned long x)
{
	gate_io((void *) &x, 0, NULL, NULL, 0, NULL, 0);
	return x;
}

int loop2(void)
{
	unsigned long i = 0;
	unsigned long saved_stack = 0;

	while (1) {
		if (saved_stack != i)
			return 1;

		if (saved_mem != i * 123)
			return 1;

		i++;
		saved_stack = barrier(i);
		saved_mem = barrier(i * 123);
	}
}
