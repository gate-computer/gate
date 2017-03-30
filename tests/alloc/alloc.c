#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

#include <gate.h>

#define ORIGIN 1 // TODO

static const int num_garbage_slots = 2;
static void *garbage[num_garbage_slots];

static void do_it(int c, int n)
{
	size_t size = sizeof (struct gate_packet) + n + 1;

	struct gate_packet *buf = calloc(size, sizeof (char));
	if (buf == NULL)
		gate_exit(1);

	buf->size = size;
	buf->code = ORIGIN;

	memset(buf + 1, c, n);
	((char *) (buf + 1))[n] = '\n';

	gate_send_packet(buf);

	while (true) {
		for (int i = 0; i < num_garbage_slots; i++)
			if (garbage[i] == NULL) {
				garbage[i] = buf;
				return;
			}

		for (int i = 0; i < num_garbage_slots; i++) {
			free(garbage[i]);
			garbage[i] = NULL;
		}
	}
}

int main(void)
{
	for (int i = 33; i < 127; i++)
		do_it(i, i - 32);

	return 0;
}
