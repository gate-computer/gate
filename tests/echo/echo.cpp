// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <stdint.h>
#include <string.h>

#include <gate.h>

#include "../discover.h"

#define CONTENT "ECHO.. Echo.. echo..\n"

struct message_op {
	gate_packet header;
	char content[sizeof (CONTENT)];
} GATE_PACKED;

struct message_ev {
	gate_packet header;
	char content[0];
} GATE_PACKED;

static void send_message()
{
	const message_op packet = {
		.header = {
			.size = sizeof (packet),
		},
		.content = CONTENT,
	};

	gate_send_packet(&packet.header);
}

static message_ev *recv_message(char *buf, size_t bufsize)
{
	auto packet = reinterpret_cast<message_ev *> (buf);

	do {
		gate_recv_packet(buf, bufsize, 0);
	} while (packet->header.code != 0);

	return packet;
}

int main()
{
	discover_service("echo");
	send_message();

	char buf[gate_max_packet_size];
	const auto reply = recv_message(buf, gate_max_packet_size);

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
