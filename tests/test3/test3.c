#include <stddef.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

#include <gate.h>

static void fail(int code)
{
	static char buf[sizeof (struct gate_op_packet) + sizeof (int32_t)];

	struct gate_op_packet *head = (struct gate_op_packet *) buf;
	head->size = sizeof (buf);
	head->code = GATE_OP_CODE_ORIGIN;

	*(int32_t *) (buf + sizeof (struct gate_op_packet)) = (int32_t) code;

	gate_send_packet(head);
	gate_exit(1);
}

static void do_it(int n)
{
	size_t size = sizeof (struct gate_op_packet) + n;

	struct gate_op_packet *buf = calloc(size, sizeof (char));
	if (buf == NULL)
		fail(n);

	buf->size = size;
	buf->code = GATE_OP_CODE_ORIGIN;

	memset(buf + 1, n, n);

	gate_send_packet(buf);

	free(buf);
}

int main(void)
{
	for (int i = 32; i < 127; i++)
		do_it(i);

	return 0;
}
