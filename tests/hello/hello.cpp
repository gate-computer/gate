// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <stdint.h>

#include <gate.h>

#include "../discover.h"

long workaround;
void (*indirection)(const gate_packet *);

namespace {

void implementation(const gate_packet *p)
{
	discover_service("origin");
	gate_send_packet(p);
}

template <int PayloadSize>
class Packet
{
	Packet(const Packet &) = delete;
	Packet &operator=(const Packet &) = delete;

public:
	enum {
		header_size  = 8,
		payload_size = PayloadSize,
		size         = header_size + payload_size,
	};

	Packet()
	{
		for (int i = 0; i < header_size; i++)
			buf[i] = 0;
	}

	char *payload()
	{
		return buf + header_size;
	}

	const gate_packet *op_data(int16_t code, uint16_t flags = 0)
	{
		gate_packet *header = reinterpret_cast<gate_packet *> (buf);
		header->size = size;
		header->flags = flags;
		header->code = code;
		return header;
	}

private:
	char buf[size];
};

} // namespace

int main()
{
	indirection = implementation;

	char str[] = "hello world\n";
	Packet<sizeof (str) - 1> p;

	for (int i = 0; i < p.payload_size; i++) {
		char c = str[i];
		if (c >= 'a' && c <= 'z')
		    c += gate_arg;
		p.payload()[i] = c;
	}

	if (p.size > gate_max_packet_size)
		return 1;

	indirection(p.op_data(0));

	return 0;
}
