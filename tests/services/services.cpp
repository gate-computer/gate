// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <stdint.h>
#include <string.h>

#include <gate.h>

#define NAMES "test1\0test2"

int main()
{
	auto op_size = sizeof (gate_service_name_packet) + sizeof (NAMES);
	char op_buf[op_size];
	memset(op_buf, 0, op_size);
	auto op = reinterpret_cast<gate_service_name_packet *> (op_buf);
	op->header.size = op_size;
	op->count = 2;
	memcpy(op->names, NAMES, sizeof (NAMES));
	gate_send_packet(&op->header);

	char ev_buf[gate_max_packet_size];
	const gate_service_info_packet *ev;
	do {
		gate_recv_packet(ev_buf, gate_max_packet_size, 0);
		ev = reinterpret_cast<gate_service_info_packet *> (ev_buf);
	} while (ev->header.code != 0 || ev->header.size == sizeof (gate_packet));

	if (ev->count != 2) {
		gate_debug("Unexpected number of service entries\n");
		return 1;
	}

	if (ev->infos[0].code != 2) {
		gate_debug("Unexpected test1 service code\n");
		return 1;
	}

	if (ev->infos[0].version != 1337) {
		gate_debug("Unexpected test1 service version\n");
		return 1;
	}

	if (ev->infos[1].code != 3) {
		gate_debug("Unexpected test2 service code\n");
		return 1;
	}

	if (ev->infos[1].version != 12765) {
		gate_debug("Unexpected test2 service version\n");
		return 1;
	}

	auto codes_size = ev->header.size - sizeof (ev->header) - 8;
	if (codes_size != ev->count * sizeof (gate_service_info)) {
		gate_debug("Inconsistent packet size\n");
		return 1;
	}

	return 0;
}
