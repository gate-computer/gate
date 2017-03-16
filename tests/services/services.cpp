#include <stdint.h>
#include <string.h>

#include <gate.h>

#define NAMES "test1\0test2"

struct ServicesOp {
	struct gate_op_header header;
	struct gate_op_services payload;
} __attribute__ ((packed));

struct ServicesEv {
	struct gate_ev_header header;
	struct gate_ev_services payload;
} __attribute__ ((packed));

int main()
{
	auto op_size = sizeof (ServicesOp) + sizeof (NAMES);
	char op_buf[op_size];
	memset(op_buf, 0, op_size);
	auto op = reinterpret_cast<ServicesOp *> (op_buf);
	op->header.size = op_size;
	op->header.code = GATE_OP_CODE_SERVICES;
	op->payload.count = 2;
	memcpy(op->payload.names, NAMES, sizeof (NAMES));
	gate_send_packet(&op->header);

	char ev_buf[gate_max_packet_size];
	const ServicesEv *ev;
	do {
		gate_recv_packet(ev_buf, gate_max_packet_size, 0);
		ev = reinterpret_cast<ServicesEv *> (ev_buf);
	} while (ev->header.code != GATE_EV_CODE_SERVICES);

	if (ev->payload.count != 2) {
		gate_debug("Unexpected number of service entries\n");
		return 1;
	}

	if (ev->payload.infos[0].atom != 1) {
		gate_debug("Unexpected test1 service atom\n");
		return 1;
	}

	if (ev->payload.infos[0].version != 1337) {
		gate_debug("Unexpected test1 service version\n");
		return 1;
	}

	if (ev->payload.infos[1].atom != 2) {
		gate_debug("Unexpected test2 service atom\n");
		return 1;
	}

	if (ev->payload.infos[1].version != 12765) {
		gate_debug("Unexpected test2 service version\n");
		return 1;
	}

	auto atoms_size = ev->header.size - sizeof (ev->header) - 8;
	if (atoms_size != ev->payload.count * sizeof (gate_service_info)) {
		gate_debug("Inconsistent packet size\n");
		return 1;
	}

	return 0;
}
