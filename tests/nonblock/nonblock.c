#include <stddef.h>

#include <gate.h>

int main(void)
{
	char idle = 0;
	char payload;

	while (1) {
		gate_debug("while\n");

		idle++;

		char buf[gate_max_packet_size];
		size_t len = gate_recv_packet(buf, gate_max_packet_size, GATE_RECV_FLAG_NONBLOCK);
		if (len == 0)
			continue;

		const struct gate_ev_header *ev = (void *) buf;
		if (ev->code != GATE_EV_CODE_ORIGIN)
			continue;

		if (len < sizeof (struct gate_ev_header) + 1)
			gate_exit(1);

		payload = buf[sizeof (struct gate_ev_header)];
		break;
	}

	size_t size = sizeof (struct gate_op_header) + 2;
	char buf[size];

	struct gate_op_header *op = (void *) buf;
	op->size = size;
	op->code = GATE_OP_CODE_ORIGIN;
	op->flags = 0;

	buf[sizeof (struct gate_op_header) + 0] = idle;
	buf[sizeof (struct gate_op_header) + 1] = payload;

	gate_send_packet(op);
	return 0;
}
