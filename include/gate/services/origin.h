#ifndef _GATE_SERVICES_ORIGIN_H
#define _GATE_SERVICES_ORIGIN_H

#include <stddef.h>
#include <string.h>

#include "../../gate.h"

#define ORIGIN_SERVICE_NAME "origin"

static inline void origin_send(uint16_t code, const void *msg, size_t msglen)
{
	if (msglen > gate_max_packet_size - sizeof (struct gate_packet))
		gate_exit(1);

	size_t size = sizeof (struct gate_packet) + msglen;
	char buf[size];
	struct gate_packet *header = (struct gate_packet *) buf;

	memset(buf, 0, sizeof (struct gate_packet));
	header->size = size;
	header->code = code;

	memcpy(buf + sizeof (struct gate_packet), msg, msglen);

	gate_send_packet(header);
}

static inline void origin_send_str(uint16_t code, const char *msg)
{
	origin_send(code, msg, strlen(msg));
}

static inline void origin_send_init(uint16_t code)
{
	origin_send(code, NULL, 0);
}

#endif
