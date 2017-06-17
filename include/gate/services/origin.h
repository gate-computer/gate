#ifndef _GATE_SERVICES_ORIGIN_H
#define _GATE_SERVICES_ORIGIN_H

#include <stddef.h>
#include <string.h>

#include "../../gate.h"

static inline void origin_send_packet(uint16_t code, const void *msg, size_t msglen)
{
	size_t size = sizeof (struct gate_packet) + msglen;
	char buf[size];
	struct gate_packet *header = (struct gate_packet *) buf;

	memset(buf, 0, sizeof (struct gate_packet));
	header->size = size;
	header->code = code;

	memcpy(buf + sizeof (struct gate_packet), msg, msglen);

	gate_send_packet(header);
}

#endif
