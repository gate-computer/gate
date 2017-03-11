#include <stdint.h>

#include <gate.h>

#define NAMES "test1\0test2"

struct Info {
	uint32_t atom;
	uint32_t version;
} __attribute__ ((packed));

struct OpPayload {
	uint32_t count;
	uint32_t dummy;
	char names[sizeof (NAMES)];
} __attribute__ ((packed));

struct Op {
	struct gate_op_header header;
	struct OpPayload payload;
} __attribute__ ((packed));

struct EvPayload {
	uint32_t count;
	uint32_t dummy;
	Info infos[];
} __attribute__ ((packed));

struct Ev {
	struct gate_ev_header header;
	struct EvPayload payload;
} __attribute__ ((packed));

int main()
{
	const Op op = {
		.header = {
			.size = sizeof (op),
			.code = GATE_OP_CODE_INTERFACES,
		},
		.payload = {
			.count = 2,
			.names = NAMES,
		},
	};
	gate_send_packet(&op.header);

	char ev_buf[gate_max_packet_size];
	gate_recv_packet(ev_buf, gate_max_packet_size, 0);
	auto ev = reinterpret_cast<const Ev *> (ev_buf);

	if (ev->header.code != GATE_EV_CODE_INTERFACES) {
		gate_debug("Unexpected packet type\n");
		return 1;
	}

	if (ev->payload.count != 2) {
		gate_debug("Unexpected number of interface entries\n");
		return 1;
	}

	if (ev->payload.infos[0].atom != 1) {
		gate_debug("Unexpected test1 interface atom\n");
		return 1;
	}

	if (ev->payload.infos[0].version != 1337) {
		gate_debug("Unexpected test1 interface version\n");
		return 1;
	}

	if (ev->payload.infos[1].atom != 2) {
		gate_debug("Unexpected test2 interface atom\n");
		return 1;
	}

	if (ev->payload.infos[1].version != 12765) {
		gate_debug("Unexpected test2 interface version\n");
		return 1;
	}

	auto atoms_size = ev->header.size - sizeof (ev->header) - 8;
	if (atoms_size != ev->payload.count * sizeof (Info)) {
		gate_debug("Inconsistent packet size\n");
		return 1;
	}

	return 0;
}
