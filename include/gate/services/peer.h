#ifndef _GATE_SERVICES_PEER_H
#define _GATE_SERVICES_PEER_H

#include <stdint.h>

#include "../../gate.h"

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

#endif
