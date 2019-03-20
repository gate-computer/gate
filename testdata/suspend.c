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

	size_t n = sizeof packet;
	gate_io(NULL, NULL, &packet, &n, 0);

	for (long i = 0;; i++)
		iteration(i);
}
