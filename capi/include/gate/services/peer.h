// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#ifndef _GATE_SERVICES_PEER_H
#define _GATE_SERVICES_PEER_H

#include <stdint.h>

#include "../../gate.h"

#define PEER_SERVICE_NAME "peer"

enum peer_op_type {
	PEER_OP_INIT,
	PEER_OP_MESSAGE,
};

enum peer_ev_type {
	PEER_EV_ERROR,
	PEER_EV_MESSAGE,
	PEER_EV_ADDED,
	PEER_EV_REMOVED,
};

struct peer_packet {
	struct gate_packet header;
	uint8_t type;
	uint8_t padding[7];
} GATE_PACKED;

struct peer_id_packet {
	struct peer_packet peer_header;
	uint64_t peer_id;
} GATE_PACKED;

static inline void peer_send_init(int16_t code)
{
	const struct peer_packet packet = {
		.header = {
			.size = sizeof (packet),
			.code = code,
		},
		.type = PEER_OP_INIT,
	};

	gate_send_packet(&packet.header, 0);
}

static inline void peer_send_message_packet(void *buf, size_t size, int16_t code, uint64_t peer_id)
{
	struct peer_id_packet *header = (struct peer_id_packet *) buf;

	memset(buf, 0, sizeof (struct peer_id_packet));
	header->peer_header.header.size = size;
	header->peer_header.header.code = code;
	header->peer_header.type = PEER_OP_MESSAGE;
	header->peer_id = peer_id;

	gate_send_packet(&header->peer_header.header, 0);
}

static inline void peer_send_message(int16_t code, uint64_t peer_id, const void *msg, size_t msglen)
{
	if (msglen > gate_max_packet_size - sizeof (struct peer_id_packet))
		gate_exit(1);

	size_t size = sizeof (struct peer_id_packet) + msglen;
	char buf[size];

	memcpy(buf + sizeof (struct peer_id_packet), msg, msglen);

	peer_send_message_packet(buf, size, code, peer_id);
}

#endif
