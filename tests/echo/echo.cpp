// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <stdint.h>
#include <string.h>

#include <gate.h>

#define SERVICE "echo"
#define CONTENT "ECHO.. Echo.. echo..\n"

struct ServiceNamePacket {
	gate_service_name_packet packet;
	char names[sizeof (SERVICE)];
} __attribute__ ((packed));

struct MessageOp {
	gate_packet header;
	char content[sizeof (CONTENT)];
} __attribute__ ((packed));

struct MessageEv {
	gate_packet header;
	char content[0];
} __attribute__ ((packed));

static uint16_t get_service_code()
{
	const ServiceNamePacket op = {
		.packet = {
			.header = {
				.size = sizeof (op),
			},
			.count = 1,
		},
		.names = SERVICE,
	};
	gate_send_packet(&op.packet.header);

	while (1) {
		char buf[gate_max_packet_size];
		gate_recv_packet(buf, gate_max_packet_size, 0);
		auto ev = reinterpret_cast<const gate_service_info_packet *> (buf);
		if (ev->header.code == 0)
			return ev->infos[0].code;
	}
}

static void send_message(uint16_t code)
{
	const MessageOp op = {
		.header = {
			.size = sizeof (op),
			.code = code,
		},
		.content = CONTENT,
	};
	gate_send_packet(&op.header);
}

static MessageEv *recv_message(char *buf, size_t bufsize, uint16_t code)
{
	while (1) {
		gate_recv_packet(buf, bufsize, 0);
		auto ev = reinterpret_cast<MessageEv *> (buf);
		if (ev->header.code == code)
			return ev;
	}
}

int main()
{
	auto code = get_service_code();
	if (code == 0) {
		gate_debug("No such service: echo\n");
		return 1;
	}

	send_message(code);

	char buf[gate_max_packet_size];
	const auto reply = recv_message(buf, gate_max_packet_size, code);

	auto length = reply->header.size - sizeof (reply->header);
	if (length != sizeof (CONTENT)) {
		gate_debug("Length mismatch\n");
		return 1;
	}

	if (memcmp(reply->content, CONTENT, length) != 0) {
		gate_debug("Content mismatch\n");
		return 1;
	}

	return 0;
}
