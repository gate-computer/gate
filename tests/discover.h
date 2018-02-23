// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <stddef.h>

#include <gate.h>

static inline void discover_service(const char *name)
{
	char buf[gate_max_packet_size];

	for (size_t i = 0; i < sizeof(struct gate_packet); i++)
		buf[i] = 0;

	struct gate_service_name_packet *op = (struct gate_service_name_packet *) buf;

	size_t n = 0;
	do {
		op->names[n] = name[n];
	} while (name[n++]);

	op->header.size = sizeof(struct gate_service_name_packet) + n;
	op->header.code = GATE_PACKET_CODE_SERVICES;
	op->count = 1;

	gate_send_packet(&op->header, 0);

	struct gate_service_info_packet *ev = (struct gate_service_info_packet *) buf;

	do {
		gate_recv_packet(buf, gate_max_packet_size, 0);
	} while (ev->header.code != GATE_PACKET_CODE_SERVICES);

	if (ev->count != 1) {
		gate_debug("Service discovery response with unexpected number of services\n");
		gate_exit(1);
	}

	if ((ev->infos[0].flags & GATE_SERVICE_FLAG_AVAILABLE) == 0) {
		gate_debug3("Service not available: ", name, "\n");
		gate_exit(1);
	}
}
