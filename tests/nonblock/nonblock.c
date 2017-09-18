// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <stddef.h>

#include <gate.h>

#include "../discover.h"

int main()
{
	discover_service("origin");

	int idle = 0;
	char payload;

	while (1) {
		idle++;

		char buf[gate_max_packet_size];
		size_t len = gate_recv_packet(buf, gate_max_packet_size, GATE_RECV_FLAG_NONBLOCK);
		if (len == 0)
			continue;

		const struct gate_packet *ev = (void *) buf;
		if (ev->code != 0)
			continue;

		if (len < sizeof (struct gate_packet) + 1)
			gate_exit(1);

		payload = buf[sizeof (struct gate_packet)];
		break;
	}

	size_t size = sizeof (struct gate_packet) + 3;
	char buf[size];

	for (size_t i = 0; i < size; i++)
		buf[i] = 0;

	struct gate_packet *op = (void *) buf;
	op->size = size;

	buf[sizeof (struct gate_packet) + 0] = idle;
	buf[sizeof (struct gate_packet) + 1] = idle >> 8;
	buf[sizeof (struct gate_packet) + 2] = payload;

	gate_send_packet(op, 0);

	return 0;
}
