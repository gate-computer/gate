// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <stdbool.h>
#include <stddef.h>
#include <stdlib.h>

#include <gate.h>
#include <gate/service.h>
#include <gate/services/origin.h>
#include <gate/services/peer.h>

static void *xalloc(size_t size)
{
	void *ptr = calloc(1, size);
	if (ptr == NULL) {
		gate_debug("out of memory\n");
		gate_exit(1);
	}
	return ptr;
}

struct message_packet {
	struct peer_id_packet peer_id_header;
	char payload[0]; // variable length
};

static size_t message_packet_size(const struct message_packet *packet)
{
	return packet->peer_id_header.peer_header.header.size;
}

static size_t message_payload_size(const struct message_packet *packet)
{
	return message_packet_size(packet) - sizeof (struct peer_id_packet);
}

static void origin_packet_received(struct gate_service *service, void *data, size_t size);
static void peer_packet_received(struct gate_service *service, void *data, size_t size);

static struct gate_service origin = {
	.name = ORIGIN_SERVICE_NAME,
	.received = origin_packet_received,
};

static struct gate_service peer = {
	.name = PEER_SERVICE_NAME,
	.received = peer_packet_received,
};

struct peer {
	uint64_t peer_id;
	struct peer *next;
};

static bool exit_requested;
static struct peer *peer_list;

static void origin_packet_received(struct gate_service *service, void *input, size_t inputsize)
{
	gate_debug("origin packet received\n");

	for (size_t i = sizeof (struct gate_packet); i < inputsize; i++) {
		if (((const char *) input)[i] == 0) {
			exit_requested = true;
			inputsize = i;
			break;
		}
	}

	void *msg = input + sizeof (struct gate_packet);
	size_t msglen = inputsize - sizeof (struct gate_packet);
	if (msglen == 0)
		return;

	size_t outputsize = sizeof (struct peer_id_packet) + msglen;
	char output[outputsize];

	struct message_packet *packet = (struct message_packet *) output;
	memcpy(packet->payload, msg, msglen);

	for (struct peer *node = peer_list; node; node = node->next)
		peer_send_message_packet(output, outputsize, peer.code, node->peer_id);
}

static void add_peer(const struct peer_id_packet *packet)
{
	origin_send_str(origin.code, "adding peer\n");

	struct peer **end;
	for (end = &peer_list; *end; end = &(*end)->next)
		if ((*end)->peer_id == packet->peer_id) {
			gate_debug("peer already exists\n");
			return;
		}

	struct peer *node = xalloc(sizeof (struct peer));
	node->peer_id = packet->peer_id;

	*end = node;
}

static void remove_peer(const struct peer_id_packet *packet)
{
	origin_send_str(origin.code, "removing peer\n");

	for (struct peer **p = &peer_list; ; p = &(*p)->next) {
		if (*p == NULL) {
			gate_debug("peer not found\n");
			return;
		}

		if ((*p)->peer_id == packet->peer_id) {
			struct peer *next = (*p)->next;
			free(*p);
			*p = next;
			break;
		}
	}
}

static void message_from_peer(const struct message_packet *packet)
{
	size_t size = message_payload_size(packet);
	if (size > 0)
		origin_send(origin.code, packet->payload, size);
	else
		origin_send_str(origin.code, "empty message from peer\n");
}

static void peer_packet_received(struct gate_service *service, void *data, size_t size)
{
	const struct peer_packet *packet = data;
	enum peer_ev_type type = packet->type;

	switch (type) {
	case PEER_EV_ERROR:
		origin_send_str(origin.code, "peer service error\n");
		gate_exit(1);
		break;

	case PEER_EV_MESSAGE:
		message_from_peer(data);
		break;

	case PEER_EV_ADDED:
		add_peer(data);
		break;

	case PEER_EV_REMOVED:
		remove_peer(data);
		break;

	default:
		origin_send_str(origin.code, "unknown peer service packet\n");
		break;
	}
}

int main()
{
	struct gate_service_registry *r = gate_service_registry_create();
	if (r == NULL)
		gate_exit(1);

	if (!gate_register_service(r, &origin))
		gate_exit(1);

	if (!gate_register_service(r, &peer))
		gate_exit(1);

	if (!gate_discover_services(r))
		gate_exit(1);

	if ((origin.flags & GATE_SERVICE_FLAG_AVAILABLE) == 0) {
		gate_debug("origin service not found\n");
		gate_exit(1);
	}

	if ((peer.flags & GATE_SERVICE_FLAG_AVAILABLE) == 0) {
		origin_send_str(origin.code, "peer service not found\n");
		gate_exit(1);
	}

	gate_debug("payload is up and running\n");
	origin_send_init(origin.code);
	peer_send_init(peer.code);

	while (!exit_requested)
		gate_recv_for_services(r, 0);

	gate_debug("payload exiting\n");

	return 0;
}
