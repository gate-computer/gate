#include <gate.h>
#include <gate/service-inline.h>

#define container_of(ptr, type, member) ({ \
	const typeof( ((type *)0)->member ) *__mptr = (ptr); \
	(type *)( (char *)__mptr - offsetof(type,member) );})

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
	uint8_t padding[3];
} GATE_PACKED;

struct peer_id_packet {
	struct peer_packet peer_header;
	uint64_t peer_id;
} GATE_PACKED;

struct peer_service {
	struct gate_service parent;

	uint64_t my_peer_id;
	bool done;
};

static void peer_packet_received(struct gate_service *parent, void *data, size_t size)
{
	struct peer_service *service = container_of(parent, struct peer_service, parent);
	const struct peer_packet *packet = data;
	const struct peer_id_packet *id_packet = data;
	enum peer_ev_type type = packet->type;

	switch (type) {
	case PEER_EV_ERROR:
		gate_debug("peer service error\n");
		gate_exit(1);
		break;

	case PEER_EV_MESSAGE:
		gate_debug("message from peer\n");
		service->done = true;
		break;

	case PEER_EV_ADDED:
		if (service->my_peer_id == 0) {
			service->my_peer_id = id_packet->peer_id;
			gate_debug("peer added\n");
		} else {
			gate_debug("another peer added\n");
		}
		break;

	case PEER_EV_REMOVED:
		if (service->my_peer_id == id_packet->peer_id) {
			service->my_peer_id = 0;
			gate_debug("peer removed\n");
		} else {
			gate_debug("another peer removed\n");
		}
		break;

	default:
		gate_debug("unknown peer service packet\n");
		break;
	}
}

static struct peer_service service = {
	.parent = {
		.name = "peer",
		.received = peer_packet_received,
	},
};

static void send_peer_init_packet(void)
{
	const struct peer_packet packet = {
		.header = {
			.size = sizeof (packet),
			.code = service.parent.code,
		},
		.type = PEER_OP_INIT,
	};

	gate_send_packet(&packet.header);
}

static void send_peer_message_packet(void)
{
	const struct peer_id_packet packet = {
		.peer_header = {
			.header = {
				.size = sizeof (packet),
				.code = service.parent.code,
			},
			.type = PEER_OP_MESSAGE,
		},
		.peer_id = service.my_peer_id,
	};

	gate_send_packet(&packet.peer_header.header);
}

int main(void)
{
	struct gate_service_registry *r = gate_service_registry_create();
	if (r == NULL)
		gate_exit(1);

	if (!gate_register_service(r, &service.parent))
		gate_exit(1);

	if (!gate_discover_services(r))
		gate_exit(1);

	if (service.parent.code == 0) {
		gate_debug("peer service not found\n");
		gate_exit(1);
	}

	send_peer_init_packet();

	bool message_sent = false;

	while (!service.done) {
		gate_recv_for_services(r, 0);

		if (service.my_peer_id && !message_sent) {
			send_peer_message_packet();
			message_sent = true;
		}
	}

	return 0;
}
