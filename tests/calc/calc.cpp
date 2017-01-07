#include <stddef.h>
#include <stdint.h>

#include <gate.h>

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
	Calculator state;

	while (1) {
		char evdata[gate_max_packet_size];
		gate_recv_packet(evdata, gate_max_packet_size, 0);
		auto evhead = reinterpret_cast<const gate_ev_header *> (evdata);

		if (evhead->code == GATE_EV_CODE_ORIGIN) {
			const Buf<const char> expr = {
				evdata + sizeof (gate_ev_header),
				evhead->size - sizeof (gate_ev_header),
			};

			if (expr.size == 0)
				break;

			char opdata[gate_max_packet_size];

			for (unsigned i = 0; i < sizeof (gate_op_header); i++)
				opdata[i] = 0;

			const Buf<char> out = {
				opdata + sizeof (gate_op_header),
				sizeof (opdata) - sizeof (gate_op_header),
			};

			auto outlen = state.eval(expr, out);

			auto ophead = reinterpret_cast<gate_op_header *> (opdata);
			ophead->size = sizeof (gate_op_header) + outlen;
			ophead->code = GATE_OP_CODE_ORIGIN;

			gate_send_packet(ophead);
		}
	}

	return 0;
}
