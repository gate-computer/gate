#include <stdint.h>
#include <string.h>

#include <gate.h>

#define SERVICE "echo"
#define CONTENT "ECHO.. Echo.. echo..\n"

struct ServicesOp {
	gate_op_header header;
	gate_op_services payload;
	char names[sizeof (SERVICE)];
} __attribute__ ((packed));

struct ServicesEv {
	gate_ev_header header;
	gate_ev_services payload;
} __attribute__ ((packed));

struct MessageOp {
	gate_op_header header;
	uint32_t service;
	char content[sizeof (CONTENT)];
} __attribute__ ((packed));

struct MessageEv {
	struct {
		gate_ev_header er;
		uint32_t service;
	} head;
	char content[0];
} __attribute__ ((packed));

static uint32_t get_service_atom()
{
	const ServicesOp op = {
		.header = {
			.size = sizeof (op),
			.code = GATE_OP_CODE_SERVICES,
		},
		.payload = {
			.count = 1,
		},
		.names = SERVICE,
	};
	gate_send_packet(&op.header);

	while (1) {
		char buf[gate_max_packet_size];
		gate_recv_packet(buf, gate_max_packet_size, 0);
		auto ev = reinterpret_cast<const ServicesEv *> (buf);
		if (ev->header.code == GATE_EV_CODE_SERVICES)
			return ev->payload.infos[0].atom;
	}
}

static void send_message(uint32_t service)
{
	const MessageOp op = {
		.header = {
			.size = sizeof (op),
			.code = GATE_OP_CODE_MESSAGE,
		},
		.service = service,
		.content = CONTENT,
	};
	gate_send_packet(&op.header);
}

static MessageEv *recv_message(char *buf, size_t bufsize, uint32_t service)
{
	while (1) {
		gate_recv_packet(buf, bufsize, 0);
		auto ev = reinterpret_cast<MessageEv *> (buf);
		if (ev->head.er.code == GATE_EV_CODE_MESSAGE && ev->head.service == service)
			return ev;
	}
}

int main()
{
	auto service = get_service_atom();

	send_message(service);

	char buf[gate_max_packet_size];
	const auto reply = recv_message(buf, gate_max_packet_size, service);

	auto length = reply->head.er.size - sizeof (reply->head);
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
