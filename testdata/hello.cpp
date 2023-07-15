// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <stdint.h>

#include <gate.h>

namespace {

gate_packet* receive_packet(void* buf, size_t bufsize)
{
	if (bufsize < GATE_MAX_RECV_SIZE)
		gate_exit(1);

	size_t offset = 0;

	while (offset < sizeof(gate_packet))
		offset += gate_recv(reinterpret_cast<void*>(reinterpret_cast<uintptr_t>(buf) + offset), sizeof(gate_packet) - offset, -1);

	auto header = reinterpret_cast<gate_packet*>(buf);
	auto aligned_size = GATE_ALIGN_PACKET(header->size);

	while (offset < aligned_size)
		offset += gate_recv(reinterpret_cast<void*>(reinterpret_cast<uintptr_t>(buf) + offset), aligned_size - offset, -1);

	return header;
}

void send(void const* data, size_t size)
{
	size_t offset = 0;

	while (offset < size)
		offset += gate_send(reinterpret_cast<void*>(reinterpret_cast<uintptr_t>(data) + offset), size - offset, -1);
}

char receive_buffer[GATE_MAX_RECV_SIZE];

int discover(int16_t* origin_code, int16_t* test_code)
{
	struct {
		gate_service_name_packet header;
		char names[12 + 7]; // Space for terminator/padding.
	} discover = {
		.header = {
			.header = {
				.size = sizeof discover - 7, // No terminator/padding.
				.code = GATE_PACKET_CODE_SERVICES,
			},
			.count = 2,
		},
		.names = "\x06origin\x04test",
	};

	send(&discover, GATE_ALIGN_PACKET(sizeof discover - 7));

	auto packet = receive_packet(receive_buffer, sizeof receive_buffer);

	if (packet->code != GATE_PACKET_CODE_SERVICES) {
		__gate_debug_str("error: expected reply packet from services\n");
		return -1;
	}

	auto discovery = reinterpret_cast<gate_service_state_packet*>(packet);

	if (discovery->count != 2) {
		__gate_debug_str("error: expected 2 service states from services\n");
		return -1;
	}

	if (discovery->states[0] & GATE_SERVICE_STATE_AVAIL)
		*origin_code = 0;
	else
		*origin_code = -1;

	if (discovery->states[1] & GATE_SERVICE_STATE_AVAIL)
		*test_code = 1;
	else
		*test_code = -1;

	return 0;
}

int32_t accept_stream(int16_t origin_code, int32_t recv_flow, int32_t* send_flow)
{
	gate_packet accept = {
		.size = sizeof accept,
		.code = origin_code,
		.domain = GATE_PACKET_DOMAIN_CALL,
	};

	send(&accept, sizeof accept);

	int32_t id = -1;
	while (1) {
		auto packet = receive_packet(receive_buffer, sizeof receive_buffer);

		if (packet->code != origin_code) {
			__gate_debug_str("error: expected packet from origin\n");
			return -1;
		}

		if (packet->domain != GATE_PACKET_DOMAIN_CALL) {
			gate_debug("received origin packet with domain ", packet->domain, " while accepting stream\n");
			continue;
		}

		struct reply_packet {
			gate_packet header;
			int32_t id;
			int32_t error;
		} GATE_PACKED* reply;

		if (packet->size != sizeof *reply) {
			__gate_debug_str("error: accept call reply has unexpected size\n");
			return -1;
		}

		reply = reinterpret_cast<reply_packet*>(packet);
		id = reply->id;
		break;
	}

	struct {
		gate_flow_packet header;
		gate_flow flows[1];
	} flow = {
		.header = {
			.header = {
				.size = sizeof flow,
				.code = origin_code,
				.domain = GATE_PACKET_DOMAIN_FLOW,
			},
		},
		.flows = {
			{
				.id = id,
				.value = recv_flow,
			},
		},
	};

	send(&flow, sizeof flow);

	while (1) {
		auto packet = receive_packet(receive_buffer, sizeof receive_buffer);

		if (packet->code != origin_code) {
			__gate_debug_str("error: expected packet from origin\n");
			return -1;
		}

		switch (packet->domain) {
		case GATE_PACKET_DOMAIN_FLOW:
			break;

		case GATE_PACKET_DOMAIN_DATA:
			if (packet->size == sizeof(gate_data_packet)) { // EOF
				continue;
			} else {
				__gate_debug_str("error: unexpected data from origin\n");
				return -1;
			}

		default:
			__gate_debug_str("error: expected flow or EOF packet from origin\n");
			return -1;
		}

		auto count = (packet->size - sizeof(gate_flow_packet)) / sizeof(gate_flow);
		auto flow_packet = reinterpret_cast<gate_flow_packet*>(packet);

		for (unsigned i = 0; i < count; i++) {
			auto flow = &flow_packet->flows[i];
			if (flow->id == id) {
				*send_flow = flow->value;
				return id;
			}
		}

		__gate_debug_str("stream not found in flow packet, waiting for another\n");
	}
}

void close_stream(int16_t origin_code, int32_t id)
{
	gate_data_packet close = {
		.header = {
			.size = sizeof close,
			.code = origin_code,
			.domain = GATE_PACKET_DOMAIN_DATA,
		},
		.id = id,
	};

	send(&close, sizeof close);
}

int send_hello(int16_t origin_code, int32_t id, int* flow)
{
	struct {
		gate_data_packet header;
		char data[13 + 7]; // Space for terminator/padding.
	} hello = {
		.header = {
			.header = {
				.size = sizeof hello - 7, // No terminator/padding.
				.code = origin_code,
				.domain = GATE_PACKET_DOMAIN_DATA,
			},
			.id = id,
		},
		.data = "hello, world\n",
	};

	if (int(sizeof hello.data) > *flow) {
		__gate_debug_str("error: not enough flow for hello\n");
		return -1;
	}

	send(&hello, GATE_ALIGN_PACKET(sizeof hello - 7));
	*flow -= sizeof hello.data;
	return 0;
}

int read_command(int16_t origin_code, int32_t id)
{
	while (1) {
		auto packet = receive_packet(receive_buffer, sizeof receive_buffer);

		if (packet->code != origin_code)
			continue;

		if (packet->domain != GATE_PACKET_DOMAIN_DATA)
			continue;

		auto datapacket = reinterpret_cast<gate_data_packet*>(packet);
		if (datapacket->id != id)
			continue;

		if (packet->size == sizeof(gate_data_packet))
			return 0;

		return 1;
	}
}

} // namespace

extern "C" {

int greet()
{
	int16_t origin_code;
	int16_t test_code;

	if (discover(&origin_code, &test_code) < 0)
		return 1;

	if (origin_code < 0) {
		gate_debug("origin service is unavailable\n");
		return 1;
	}

	int flow;
	int32_t id = accept_stream(origin_code, 0, &flow);
	if (id < 0)
		return 1;

	if (send_hello(origin_code, id, &flow) == 0)
		return 0;

	return 1;
}

int twice()
{
	int16_t origin_code;
	int16_t test_code;

	if (discover(&origin_code, &test_code) < 0)
		return 1;

	if (origin_code < 0) {
		gate_debug("origin service is unavailable\n");
		return 1;
	}

	int flow;
	int32_t id = accept_stream(origin_code, 0, &flow);
	if (id < 0)
		return 1;

	if (send_hello(origin_code, id, &flow) == 0)
		if (send_hello(origin_code, id, &flow) == 0)
			return 0;

	return 1;
}

void multi()
{
	int16_t origin_code;
	int16_t test_code;

	if (discover(&origin_code, &test_code) < 0)
		gate_exit(1);

	if (origin_code < 0) {
		gate_debug("origin service is unavailable\n");
		gate_exit(1);
	}

	while (1) {
		gate_debug("multi: accepting stream\n");

		int flow;
		int32_t id = accept_stream(origin_code, 0, &flow);
		if (id < 0)
			gate_exit(1);

		gate_debug("multi: greeting connection\n");

		if (send_hello(origin_code, id, &flow) < 0)
			gate_exit(1);

		close_stream(origin_code, id);
	}
}

int repl()
{
	int16_t origin_code;
	int16_t test_code;

	if (discover(&origin_code, &test_code) < 0)
		return 1;

	if (origin_code < 0) {
		gate_debug("origin service is unavailable\n");
		return 1;
	}

	int flow;
	int32_t id = accept_stream(origin_code, 4096, &flow);
	if (id < 0)
		return 1;

	gate_debug("repl: connection accepted\n");

	while (1)
		switch (read_command(origin_code, id)) {
		case -1:
			return 1;

		case 0:
			return 0;

		default:
			gate_debug("repl: command\n");
			if (send_hello(origin_code, id, &flow) < 0)
				return 1;
		}
}

int fail()
{
	gate_debug("exiting with return value 1\n");
	gate_exit(1);
}

int test_ext()
{
	int16_t origin_code;
	int16_t test_code;

	if (discover(&origin_code, &test_code) < 0)
		return 1;

	if (test_code < 0) {
		gate_debug("test service is unavailable\n");
		return 1;
	}

	struct {
		gate_packet header;
		uint64_t data;
	} req = {
		.header = {
			.size = sizeof req,
			.code = test_code,
		},
		.data = 0x0102030405060708,
	};

	send(&req, sizeof req);

	auto resp = receive_packet(receive_buffer, sizeof receive_buffer);

	if (resp->code != test_code) {
		__gate_debug_str("error: expected reply packet from test service\n");
		return 1;
	}

	if (*reinterpret_cast<uint64_t*>(resp + 1) != req.data) {
		__gate_debug_str("error: incorrect data in reply\n");
		return 1;
	}

	return 0;
}

void* memset(void* s, int c, size_t n)
{
	for (size_t i = 0; i < n; i++)
		reinterpret_cast<uint8_t*>(s)[i] = c;
	return s;
}

} // extern "C"
