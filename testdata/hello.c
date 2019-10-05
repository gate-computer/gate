// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <gate.h>

void *memset(void *s, int c, size_t n)
{
	for (size_t i = 0; i < n; i++)
		((char *) s)[i] = c;
	return s;
}

static struct gate_packet *receive_packet(void *buf, size_t bufsize)
{
	if (bufsize < GATE_MAX_RECV_SIZE)
		gate_exit(1);

	size_t offset = 0;

	while (offset < sizeof(struct gate_packet))
		offset += gate_recv(buf + offset, sizeof(struct gate_packet) - offset, GATE_IO_WAIT);

	struct gate_packet *header = buf;
	size_t aligned_size = GATE_ALIGN_PACKET(header->size);

	while (offset < aligned_size)
		offset += gate_recv(buf + offset, aligned_size - offset, GATE_IO_WAIT);

	return header;
}

static void send(const void *data, size_t size)
{
	size_t offset = 0;

	while (offset < size)
		offset += gate_send(data + offset, size - offset); // Busyloop :(
}

static char receive_buffer[GATE_MAX_RECV_SIZE];

static int discover(int16_t *origin_code, int16_t *test_code)
{
	struct {
		struct gate_service_name_packet header;
		char names[12];
	} discover = {
		.header = {
			.header = {
				.size = sizeof discover,
				.code = GATE_PACKET_CODE_SERVICES,
			},
			.count = 2,
		},
		.names = "origin\0test",
	};

	// Send some uninitialized bytes from stack as padding.
	send(&discover, GATE_ALIGN_PACKET(sizeof discover));

	struct gate_packet *packet = receive_packet(receive_buffer, sizeof receive_buffer);

	if (packet->code != GATE_PACKET_CODE_SERVICES) {
		__gate_debug_str("error: expected reply packet from services\n");
		return -1;
	}

	struct gate_service_state_packet *discovery = (struct gate_service_state_packet *) packet;

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

static int accept_stream(int16_t origin_code, int32_t id, int32_t accept_flow)
{
	struct {
		struct gate_flow_packet header;
		struct gate_flow flows[1];
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
				.increment = accept_flow,
			},
		},
	};

	send(&flow, sizeof flow);

	while (1) {
		struct gate_packet *packet = receive_packet(receive_buffer, sizeof receive_buffer);

		if (packet->code != origin_code) {
			__gate_debug_str("error: expected packet from origin\n");
			return -1;
		}

		switch (packet->domain) {
		case GATE_PACKET_DOMAIN_FLOW:
			break;

		case GATE_PACKET_DOMAIN_DATA:
			if (packet->size == sizeof(struct gate_data_packet)) { // EOF
				continue;
			} else {
				__gate_debug_str("error: unexpected data from origin\n");
				return -1;
			}

		default:
			__gate_debug_str("error: expected flow or EOF packet from origin\n");
			return -1;
		}

		int count = (packet->size - sizeof(struct gate_flow_packet)) / sizeof(struct gate_flow);
		struct gate_flow_packet *flow_packet = (struct gate_flow_packet *) packet;

		for (int i = 0; i < count; i++) {
			struct gate_flow *flow = &flow_packet->flows[i];
			if (flow->id == id)
				return flow->increment;
		}

		__gate_debug_str("stream not found in flow packet, waiting for another\n");
	}
}

static void close_stream(int16_t origin_code, int32_t id)
{
	struct gate_data_packet close = {
		.header = {
			.size = sizeof close,
			.code = origin_code,
			.domain = GATE_PACKET_DOMAIN_DATA,
		},
		.id = id,
	};

	send(&close, sizeof close);
}

static int send_hello(int16_t origin_code, int32_t id, int *flow)
{
	struct {
		struct gate_data_packet header;
		char data[13];
	} hello = {
		.header = {
			.header = {
				.size = sizeof hello,
				.code = origin_code,
				.domain = GATE_PACKET_DOMAIN_DATA,
			},
			.id = id,
		},
		.data = "hello, world\n",
	};

	if ((int) sizeof hello.data > *flow) {
		__gate_debug_str("error: not enough flow for hello\n");
		return -1;
	}

	// Send some uninitialized bytes from stack as padding.
	send(&hello, GATE_ALIGN_PACKET(sizeof hello));
	*flow -= sizeof hello.data;
	return 0;
}

static int read_command(int16_t origin_code, int32_t id)
{
	while (1) {
		struct gate_packet *packet = receive_packet(receive_buffer, sizeof receive_buffer);

		if (packet->code != origin_code)
			continue;

		if (packet->domain != GATE_PACKET_DOMAIN_DATA)
			continue;

		struct gate_data_packet *datapacket = (struct gate_data_packet *) packet;
		if (datapacket->id != id)
			continue;

		if (packet->size == sizeof(struct gate_data_packet))
			return 0;

		return 1;
	}
}

int greet(void)
{
	int16_t origin_code;
	int16_t test_code;

	if (discover(&origin_code, &test_code) < 0)
		return 1;

	if (origin_code < 0) {
		gate_debug("origin service is unavailable\n");
		return 1;
	}

	int flow = accept_stream(origin_code, 0, 0);
	if (flow < 0)
		return 1;

	if (send_hello(origin_code, 0, &flow) == 0)
		return 0;

	return 1;
}

int twice(void)
{
	int16_t origin_code;
	int16_t test_code;

	if (discover(&origin_code, &test_code) < 0)
		return 1;

	if (origin_code < 0) {
		gate_debug("origin service is unavailable\n");
		return 1;
	}

	int flow = accept_stream(origin_code, 0, 0);
	if (flow < 0)
		return 1;

	if (send_hello(origin_code, 0, &flow) == 0)
		if (send_hello(origin_code, 0, &flow) == 0)
			return 0;

	return 1;
}

void multi(void)
{
	int16_t origin_code;
	int16_t test_code;

	if (discover(&origin_code, &test_code) < 0)
		gate_exit(1);

	if (origin_code < 0) {
		gate_debug("origin service is unavailable\n");
		gate_exit(1);
	}

	for (int32_t id = 0;; id++) {
		gate_debug("multi: accepting stream\n");

		int flow = accept_stream(origin_code, id, 0);
		if (flow < 0)
			gate_exit(1);

		gate_debug("multi: greeting connection\n");

		if (send_hello(origin_code, id, &flow) < 0)
			gate_exit(1);

		close_stream(origin_code, id);
	}
}

int repl(void)
{
	int16_t origin_code;
	int16_t test_code;

	if (discover(&origin_code, &test_code) < 0)
		return 1;

	if (origin_code < 0) {
		gate_debug("origin service is unavailable\n");
		return 1;
	}

	int flow = accept_stream(origin_code, 0, 4096);
	if (flow < 0)
		return 1;

	gate_debug("repl: connection accepted\n");

	while (1)
		switch (read_command(origin_code, 0)) {
		case -1:
			return 1;

		case 0:
			return 0;

		default:
			gate_debug("repl: command\n");
			if (send_hello(origin_code, 0, &flow) < 0)
				return 1;
		}
}

int fail(void)
{
	gate_debug("exiting with return value 1\n");
	gate_exit(1);
}

int test_plugin(void)
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
		struct gate_packet header;
		uint64_t data;
	} req = {
		.header = {
			.size = sizeof req,
			.code = test_code,
		},
		.data = 0x0102030405060708,
	};

	send(&req, sizeof req);

	struct gate_packet *resp = receive_packet(receive_buffer, sizeof receive_buffer);

	if (resp->code != test_code) {
		__gate_debug_str("error: expected reply packet from test service\n");
		return 1;
	}

	if (*(uint64_t *) (resp + 1) != req.data) {
		__gate_debug_str("error: incorrect data in reply\n");
		return 1;
	}

	return 0;
}
