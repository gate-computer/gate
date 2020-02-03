// Copyright (c) 2020 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <cstddef>
#include <iostream>
#include <vector>

#include <gate.h>

#include "localhost_generated.h"

using namespace flatbuffers;
using namespace localhost::flat;
using namespace std;

namespace {

class Packet;
void send_packet_code(Packet p, int16_t code, int domain);
Packet receive_packet(bool one = false);

class Packet {
	friend void send_packet_code(Packet p, int16_t code, int domain);
	friend Packet receive_packet(bool);

	vector<char> buf;
	size_t iolen;

public:
	explicit Packet(size_t contentsize = 0): buf(sizeof(struct gate_packet) + contentsize), iolen(0) {}

	size_t size() { return buf.size() - sizeof(struct gate_packet); }
	const char* content() const { return &buf[sizeof(struct gate_packet)]; }
	char* content() { return &buf[sizeof(struct gate_packet)]; }

	int16_t code() const { return *reinterpret_cast<const int16_t*> (&buf[4]); }
	uint8_t domain() const { return *reinterpret_cast<const uint8_t*> (&buf[6]); }

	template <typename T>
	void append(const T& x)
	{
		auto n = buf.size();
		buf.resize(n + sizeof(x));
		*reinterpret_cast<T*> (&buf[n]) = x;
	}

	template <typename T>
	void extend(const T& x)
	{
		auto n = buf.size();
		buf.resize(n + x.size());
		copy(x.begin(), x.end(), buf.begin() + n);
	}

	template <typename T>
	void extend(const T* items, size_t count)
	{
		auto n = buf.size();
		buf.resize(n + count);
		copy(items, items + count, buf.begin() + n);
	}

	void extend(const char* s) { extend(string(s)); }
};

int16_t origin_code = -32768;
int16_t localhost_code = -32768;

vector<Packet> send_queue;
vector<Packet> recv_queue;

void send_packet_code(Packet p, int16_t code, int domain)
{
	*reinterpret_cast<uint32_t*> (&p.buf[0]) = p.buf.size();
	*reinterpret_cast<int16_t*> (&p.buf[4]) = code;
	p.buf[6] = domain;
	p.buf[7] = 0;

	p.buf.resize(GATE_ALIGN_PACKET(p.buf.size()));

	send_queue.push_back(p);
}

void send_packet(Packet p, string service, int domain)
{
	int16_t code;
	if (service == "origin") {
		if (origin_code < 0)
			return;
		code = origin_code;
	} else if (service == "localhost") {
		if (localhost_code < 0)
			return;
		code = localhost_code;
	} else {
		gate_debug("unknown service\n");
		gate_exit(1);
	}

	send_packet_code(p, code, domain);
}

void update_services(const Packet& p)
{
	switch (p.domain()) {
	case GATE_PACKET_DOMAIN_CALL:
	case GATE_PACKET_DOMAIN_INFO:
		auto count = *reinterpret_cast<const uint16_t*> (&p.content()[0]);

		if (count > 0 && (p.content()[2+0] & GATE_SERVICE_STATE_AVAIL) != 0)
			origin_code = 0;
		else
			origin_code = -32768;

		if (count > 1 && p.content()[2+1] & GATE_SERVICE_STATE_AVAIL)
			localhost_code = 1;
		else
			localhost_code = -32768;

		break;
	}
}

Packet receive_packet(bool one)
{
	while (true) {
		while (!recv_queue.empty()) {
			auto p = recv_queue.begin();
			if (p->iolen < p->buf.size())
				break;

			auto result = *p;
			recv_queue.erase(p);

			auto head = reinterpret_cast<const struct gate_packet*> (&result.buf[0]);
			result.buf.resize(head->size);
			result.iolen = 0;

			if (result.code() == GATE_PACKET_CODE_SERVICES) {
				update_services(result);
				if (one)
					return result;
				continue;
			}

			return result;
		}

		if (recv_queue.empty())
			recv_queue.push_back(Packet(GATE_MAX_RECV_SIZE - sizeof(struct gate_packet)));

		const struct gate_iovec recv = {
			&recv_queue.back().buf[recv_queue.back().iolen],
			recv_queue.back().buf.size() - recv_queue.back().iolen,
		};

		struct gate_iovec send;
		int sendnum = 0;
		if (!send_queue.empty()) {
			auto& p = send_queue.front();
			send.iov_base = &p.buf[p.iolen];
			send.iov_len = p.buf.size() - p.iolen;
			sendnum = 1;
		}

		size_t recvlen;
		size_t sentlen;
		gate_io(&recv, 1, &recvlen, &send, sendnum, &sentlen, GATE_IO_WAIT);

		if (sentlen > 0) {
			auto p = send_queue.begin();
			p->iolen += sentlen;

			if (p->iolen == p->buf.size())
				send_queue.erase(p);
		}

		if (recvlen > 0) {
			auto p = &recv_queue.back();
			p->iolen += recvlen;

			while (p->iolen >= sizeof(struct gate_packet)) {
				auto head = reinterpret_cast<const struct gate_packet*> (&p->buf[0]);

				if (p->buf.size() == GATE_ALIGN_PACKET(head->size))
					break;

				if (p->iolen <= GATE_ALIGN_PACKET(head->size)) {
					p->buf.resize(GATE_ALIGN_PACKET(head->size));
					break;
				}

				Packet tail(GATE_MAX_RECV_SIZE - sizeof(struct gate_packet));
				tail.iolen = p->iolen - GATE_ALIGN_PACKET(head->size);
				memcpy(&tail.buf[0], &p->buf[GATE_ALIGN_PACKET(head->size)], tail.iolen);

				p->iolen = GATE_ALIGN_PACKET(head->size);
				p->buf.resize(head->size);

				recv_queue.push_back(tail);

				p = &recv_queue.back();
			}
		}
	}
}

__attribute__ ((constructor))
void init_services()
{
	Packet p;
	p.append(uint16_t(2));
	p.extend("origin");
	p.append('\0');
	p.extend("gate.computer/localhost");
	p.append('\0');
	send_packet_code(p, GATE_PACKET_CODE_SERVICES, GATE_PACKET_DOMAIN_CALL);
	receive_packet(true);
}

__attribute__ ((destructor))
void flush_packets()
{
	while (!send_queue.empty())
		receive_packet();
}

void send_data(const char *service, int32_t id, int32_t& flow, bool closed, vector<char>& buf) {
	int n = buf.size();
	if (n > flow)
		n = flow;

	if (n > 0) {
		Packet p;
		p.append(int32_t(id));
		p.append(int32_t(0)); // Note
		for (auto i = 0; i < n; i++) {
			p.append(buf[0]);
			buf.erase(buf.begin());
		}
		send_packet(p, service, GATE_PACKET_DOMAIN_DATA);
		flow -= n;
	}

	if (!buf.empty() && closed) {
		gate_debug("origin closed\n");
		gate_exit(1);
	}
}

void main_loop()
{
	bool called = false;
	vector<char> output;

	// HTTP request.
	{
		HTTPRequestT req;
		req.method = "GET";
		req.uri = "/";

		CallT call;
		call.function.Set(req);

		FlatBufferBuilder build;
		build.Finish(CreateCall(build, &call));

		Packet p;
		p.extend(build.GetBufferPointer(), build.GetSize());
		send_packet(p, "localhost", GATE_PACKET_DOMAIN_CALL);
	}

	// Accept stream.
	{
		Packet p;
		send_packet(p, "origin", GATE_PACKET_DOMAIN_CALL);
	}
	int32_t stream_id = -1;
	int32_t stream_flow = 0;
	bool stream_closed = false;

	while (!(called && output.empty())) {
		Packet received = receive_packet();

		if (received.code() == origin_code) {
			switch (received.domain()) {
			case GATE_PACKET_DOMAIN_CALL: // Stream accepted.
				stream_id = *reinterpret_cast<const int32_t*> (&received.content()[0]);
				stream_flow = 0;
				break;

			case GATE_PACKET_DOMAIN_FLOW:
				if (*reinterpret_cast<const int32_t*> (&received.content()[0]) == stream_id) {
					auto n = *reinterpret_cast<const int32_t*> (&received.content()[4]);
					stream_flow += n;
					if (n == 0)
						stream_closed = true;
				}

				send_data("origin", stream_id, stream_flow, stream_closed, output);
				break;
			}
		} else if (received.code() == localhost_code) {
			switch (received.domain()) {
			case GATE_PACKET_DOMAIN_CALL:
				called = true;

				HTTPResponseT res;
				GetRoot<HTTPResponse>(received.content())->UnPackTo(&res);
				output.resize(res.body.size());
				copy(res.body.begin(), res.body.end(), output.begin());

				send_data("origin", stream_id, stream_flow, stream_closed, output);
				break;
			}
		}
	}
}

} // namespace

int main(void)
{
	main_loop();
	return 0;
}
