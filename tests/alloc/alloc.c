// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

#include <gate.h>

#include "../discover.h"

#define NUM_GARBAGE_SLOTS 2

static void *garbage[NUM_GARBAGE_SLOTS];

static void do_it(int c, int n)
{
	size_t size = sizeof (struct gate_packet) + n + 1;

	struct gate_packet *buf = calloc(size, sizeof (char));
	if (buf == NULL)
		gate_exit(1);

	buf->size = size;

	memset(buf + 1, c, n);
	((char *) (buf + 1))[n] = '\n';

	gate_send_packet(buf, 0);

	while (true) {
		for (int i = 0; i < NUM_GARBAGE_SLOTS; i++)
			if (garbage[i] == NULL) {
				garbage[i] = buf;
				return;
			}

		for (int i = 0; i < NUM_GARBAGE_SLOTS; i++) {
			free(garbage[i]);
			garbage[i] = NULL;
		}
	}
}

int main()
{
	discover_service("origin");

	for (int i = 33; i < 127; i++)
		do_it(i, i - 32);

	return 0;
}
