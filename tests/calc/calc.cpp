// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <cstddef>
#include <cstdint>

#include <gate.h>

#include "../discover.h"

template <typename T>
class Buf {
public:
	Buf():
		data(nullptr),
		size(0)
	{
	}

	Buf(T *data, size_t size):
		data(data),
		size(size)
	{
	}

	Buf(const Buf &other):
		data(other.data),
		size(other.size)
	{
	}

	T *data;
	size_t size;
};

class Calculator {
	Calculator(const Calculator &) = delete;
	void operator=(const Calculator &) = delete;

public:
	Calculator():
		m_value(0)
	{
	}

	size_t eval(Buf<const char> expr, Buf<char> out);

private:
	int64_t m_value;
};

size_t Calculator::eval(Buf<const char> expr, Buf<char> out)
{
	bool ok = true;

	for (unsigned i = 0; i < expr.size && ok; i++) {
		switch (expr.data[i]) {
		case '+':
			m_value++;
			break;

		case '-':
			m_value--;
			break;

		default:
			ok = false;
			break;
		}
	}

	if (!ok)
		return 0;

	Buf<int64_t> values = {
		reinterpret_cast<int64_t *> (out.data),
		out.size / sizeof (int64_t),
	};

	if (values.size < 1)
		gate_exit(1);

	values.data[0] = m_value;
	return sizeof (values.data[0]);
}

int main()
{
	discover_service("origin");

	Calculator state;

	while (1) {
		char evdata[gate_max_packet_size];
		gate_recv_packet(evdata, gate_max_packet_size, 0);
		auto evhead = reinterpret_cast<const gate_packet *> (evdata);

		if (evhead->code == 0) {
			const Buf<const char> expr = {
				evdata + sizeof (gate_packet),
				evhead->size - sizeof (gate_packet),
			};

			if (expr.size == 0)
				break;

			char opdata[gate_max_packet_size];

			for (unsigned int i = 0; i < sizeof (gate_packet); i++)
				opdata[i] = 0;

			const Buf<char> out = {
				opdata + sizeof (gate_packet),
				sizeof (opdata) - sizeof (gate_packet),
			};

			auto outlen = state.eval(expr, out);

			auto ophead = reinterpret_cast<gate_packet *> (opdata);
			ophead->size = sizeof (gate_packet) + outlen;

			gate_send_packet(ophead, 0);
		}
	}

	return 0;
}
