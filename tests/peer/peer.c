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
	char content[0]; // variable length
} GATE_PACKED;

struct peer_service {
	struct gate_service service;
	int dummy;
};

static void peer_service_discovered(struct gate_service *service)
{
	struct peer_service *s = container_of(service, struct peer_service, service);

	gate_debug("discovered ");
	gate_debug(s->service.name);
	gate_debug(" service\n");
}

static void peer_packet_received(struct gate_service *service, void *data, size_t size)
{
	struct peer_service *s = container_of(service, struct peer_service, service);

	gate_debug("received ");
	gate_debug(s->service.name);
	gate_debug(" message\n");
}

static struct peer_service peer_service = {
	.service = {
		.name = "peer",
		.discovered = peer_service_discovered,
		.received = peer_packet_received,
	},
	.dummy = 1337,
};

static void send_peer_init_packet(void)
{
	const struct peer_packet packet = {
		.header = {
			.size = sizeof (packet),
			.code = peer_service.service.code,
		},
		.type = PEER_OP_INIT,
	};

	gate_send_packet(&packet.header);
}

int main(void)
{
	struct gate_service_registry *r = gate_service_registry_create();
	if (r == NULL)
		gate_exit(1);

	if (!gate_register_service(r, &peer_service.service))
		gate_exit(1);

	if (!gate_discover_services(r))
		gate_exit(1);

	if (!peer_service.service.code) {
		gate_debug("peer service not found\n");
		gate_exit(1);
	}

	send_peer_init_packet();

	return 0;
}
