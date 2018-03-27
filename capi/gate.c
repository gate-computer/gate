// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <stddef.h>
#include <stdint.h>

#include <gate.h>

#define PACKET_FLAG_SYNC 0x1

#define PACKET_CODE_LOOPBACK -3

struct loopback_packet {
	struct gate_packet header;
	uint64_t marker;
} GATE_PACKED;

extern void __gate_recv(void *buf, size_t size);
extern void __gate_send(const void *data, size_t size);

static long send_buflen;
static uint64_t recv_markers;

static void recv_packet(struct gate_packet *buf, long io_maxsize)
{
	__gate_recv(buf, sizeof(struct gate_packet));
	__gate_recv(buf + 1, buf->size - sizeof(struct gate_packet));

	if (buf->__flags & PACKET_FLAG_SYNC)
		send_buflen -= io_maxsize;
}

void __gate_recv_packet(struct gate_packet *buf)
{
	long io_maxsize = __gate_get_max_packet_size();

	do {
		recv_packet(buf, io_maxsize);
	} while (buf->code == PACKET_CODE_LOOPBACK);
}

size_t __gate_recv_packet_nonblock(struct gate_packet *buf)
{
	long io_maxsize = __gate_get_max_packet_size();
	uint64_t my_marker = 0;

	while (1) {
		long syncgap = io_maxsize - send_buflen;

		if (syncgap > 0 && my_marker == 0) {
			my_marker = ++recv_markers;

			struct loopback_packet packet = {
				.header = {
					.size = sizeof packet,
					.code = PACKET_CODE_LOOPBACK,
				},
				.marker = my_marker,
			};

			if (syncgap <= (long) packet.header.size)
				packet.header.__flags = PACKET_FLAG_SYNC;

			send_buflen += (long) packet.header.size;
			__gate_send(&packet, packet.header.size);
		}

		recv_packet(buf, io_maxsize);

		if (buf->code != PACKET_CODE_LOOPBACK)
			return buf->size;

		if (((struct loopback_packet *) buf)->marker == my_marker)
			return 0;
	}
}

static void send_packet(
	const struct gate_packet *data, long io_maxsize, long pipe_bufsize)
{
	long before = send_buflen;
	send_buflen += (long) data->size;
	long after = send_buflen;

	// TODO: modular arithmetic
	if ((before < io_maxsize && after >= io_maxsize) ||
	    (before < pipe_bufsize && after >= pipe_bufsize))
		((struct gate_packet *) data)->__flags = PACKET_FLAG_SYNC;

	__gate_send(data, data->size);
	((struct gate_packet *) data)->__flags = 0;
}

void __gate_send_packet(const struct gate_packet *data)
{
	size_t io_maxsize = __gate_get_max_packet_size();
	long pipe_bufsize = io_maxsize << 1;

	send_packet(data, io_maxsize, pipe_bufsize);
}

size_t __gate_send_packet_nonblock(const struct gate_packet *data)
{
	size_t io_maxsize = __gate_get_max_packet_size();
	long pipe_bufsize = io_maxsize << 1;
	long avail = pipe_bufsize - send_buflen;

	if ((long) data->size > avail)
		return 0;

	send_packet(data, io_maxsize, pipe_bufsize);
	return data->size;
}

size_t __gate_nonblock_send_size()
{
	size_t io_maxsize = __gate_get_max_packet_size();
	long pipe_bufsize = io_maxsize << 1;
	long avail = pipe_bufsize - send_buflen;

	if (avail >= (long) io_maxsize)
		return io_maxsize;
	else if (avail >= (long) sizeof(struct gate_packet))
		return avail;
	else
		return 0;
}
