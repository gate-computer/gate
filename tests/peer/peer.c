#include <string.h>

#include <gate.h>
#include <gate/service-inline.h>
#include <gate/services/origin.h>
#include <gate/services/peer.h>

#define container_of(ptr, type, member) ({ \
	const typeof( ((type *)0)->member ) *__mptr = (ptr); \
	(type *)( (char *)__mptr - offsetof(type,member) );})

static void origin_packet_received(struct gate_service *service, void *data, size_t size)
{
	gate_debug("origin packet received\n");
}

static struct gate_service origin_service = {
	.name = ORIGIN_SERVICE_NAME,
	.received = origin_packet_received,
};

static void send_origin_packet(const char *msg)
{
	gate_debug(msg);

	if (origin_service.code)
		origin_send_packet(origin_service.code, msg, strlen(msg));
}

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
		send_origin_packet("peer service error\n");
		gate_exit(1);
		break;

	case PEER_EV_MESSAGE:
		send_origin_packet("message from peer\n");
		service->done = true;
		break;

	case PEER_EV_ADDED:
		if (service->my_peer_id == 0) {
			service->my_peer_id = id_packet->peer_id;
			send_origin_packet("peer added\n");
		} else {
			send_origin_packet("another peer added\n");
		}
		break;

	case PEER_EV_REMOVED:
		if (service->my_peer_id == id_packet->peer_id) {
			service->my_peer_id = 0;
			send_origin_packet("peer removed\n");
		} else {
			send_origin_packet("another peer removed\n");
		}
		break;

	default:
		send_origin_packet("unknown peer service packet\n");
		break;
	}
}

static struct peer_service peer_service = {
	.parent = {
		.name = PEER_SERVICE_NAME,
		.received = peer_packet_received,
	},
};

static void send_peer_init_packet()
{
	peer_send_init_packet(peer_service.parent.code);
}

static void send_peer_message_packet()
{
	peer_send_message_packet(peer_service.parent.code, peer_service.my_peer_id);
}

void main()
{
	struct gate_service_registry *r = gate_service_registry_create();
	if (r == NULL)
		gate_exit(1);

	if (!gate_register_service(r, &peer_service.parent))
		gate_exit(1);

	gate_register_service(r, &origin_service);

	if (!gate_discover_services(r))
		gate_exit(1);

	if (peer_service.parent.code == 0) {
		send_origin_packet("peer service not found\n");
		gate_exit(1);
	}

	send_peer_init_packet();

	bool message_sent = false;

	while (!peer_service.done) {
		gate_recv_for_services(r, 0);

		if (peer_service.my_peer_id && !message_sent) {
			send_peer_message_packet();
			message_sent = true;
		}
	}
}
