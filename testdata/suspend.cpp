// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <stddef.h>
#include <stdint.h>

#include <gate.h>

namespace {

void slow_nop()
{
	gate_io(nullptr, 0, nullptr, nullptr, 0, nullptr, 0, nullptr);
}

void delay()
{
	for (int i = 0; i < 1000000; i++)
		slow_nop();
}

void iteration(long i)
{
	gate_debug("suspend.cpp running: ", i, "\n");
	delay();
}

volatile unsigned long saved_mem = 0;

__attribute__((noinline)) unsigned long barrier(unsigned long x)
{
	gate_io(reinterpret_cast<gate_iovec*>(&x), 0, nullptr, nullptr, 0, nullptr, 0, nullptr);
	return x;
}

} // namespace

extern "C" {

int loop()
{
	struct {
		gate_service_name_packet header;
		char names[25 + 1]; // Space for terminator.
	} packet = {
		.header = {
			.header = {
				.size = sizeof packet - 1, // No terminator.
				.code = GATE_PACKET_CODE_SERVICES,
			},
			.count = 3,
		},
		.names = "\x06origin\x04test\x0c_nonexistent",
	};

	// Send some uninitialized bytes from stack as padding.
	gate_send(&packet, GATE_ALIGN_PACKET(sizeof packet), -1);

	for (long i = 0;; i++)
		iteration(i);
}

int loop2()
{
	unsigned long i = 0;
	unsigned long saved_stack = 0;

	while (1) {
		if (saved_stack != i)
			return 1;

		if (saved_mem != i * 123)
			return 1;

		i++;
		saved_stack = barrier(i);
		saved_mem = barrier(i * 123);
	}
}

void* memcpy(void* dest, void const* src, size_t n)
{
	auto d = reinterpret_cast<uint8_t*>(dest);
	auto s = reinterpret_cast<uint8_t const*>(src);
	for (size_t i = 0; i < n; i++)
		d[i] = s[i];
	return dest;
}

void* memset(void* s, int c, size_t n)
{
	for (size_t i = 0; i < n; i++)
		reinterpret_cast<uint8_t*>(s)[i] = c;
	return s;
}

} // extern "C"
