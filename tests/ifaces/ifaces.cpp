#include <stdint.h>
#include <string.h>

#include <gate.h>

struct payload {
	uint32_t count;
	char strings[0];
} __attribute__ ((packed));

int main()
{
	const gate_op_header op = {
		.size = sizeof (gate_op_header),
		.code = GATE_OP_CODE_INTERFACES,
	};

	gate_send_packet(&op);

	char buf[gate_max_packet_size];
	gate_recv_packet(buf, gate_max_packet_size, 0);

	auto ifaces = reinterpret_cast<const payload *> (buf + sizeof (gate_ev_header));
	const char *names[ifaces->count];
	const char *ptr = ifaces->strings;

	for (unsigned int i = 0; i < ifaces->count; i++) {
		names[i] = ptr;
		ptr += strlen(ptr) + 1;
	}

	for (unsigned int i = 0; i < ifaces->count; i++) {
		gate_debug("Interface: '");
		gate_debug(names[i]);
		gate_debug("'\n");
	}

	if (ifaces->count == 0)
		gate_debug("No interfaces\n");

	return 0;
}
