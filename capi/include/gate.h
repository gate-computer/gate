// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#ifndef _GATE_H
#define _GATE_H

#include <stddef.h>
#include <stdint.h>

#ifndef GATE_CONSTFUNC
# define GATE_CONSTFUNC __attribute__ ((const))
#endif

#ifndef GATE_PURE
# define GATE_PURE __attribute__ ((pure))
#endif

#ifndef GATE_NORETURN
# define GATE_NORETURN __attribute__ ((noreturn))
#endif

#ifndef GATE_PACKED
# define GATE_PACKED __attribute__ ((packed))
#endif

#ifdef __cplusplus
extern "C" {
# ifndef GATE_NOEXCEPT
#  define GATE_NOEXCEPT noexcept
# endif
#else
# ifndef GATE_NOEXCEPT
#  define GATE_NOEXCEPT
# endif
#endif

#define GATE_RECV_FLAG_NONBLOCK 0x1
#define GATE_SEND_FLAG_NONBLOCK 0x1

#define GATE_PACKET_CODE_NOTHING  -1
#define GATE_PACKET_CODE_SERVICES -2

#define GATE_SERVICE_FLAG_AVAILABLE 0x1

enum gate_func_id {
	__GATE_FUNC_RESERVED
};

struct gate_packet {
	uint32_t size;
	uint8_t __flags;
	uint8_t __reserved;
	int16_t code;
} GATE_PACKED;

struct gate_service_info {
	uint8_t flags;
	uint8_t __reserved[3];
	int32_t version;
} GATE_PACKED;

struct gate_service_name_packet {
	struct gate_packet header;
	uint32_t __reserved;
	uint16_t count;
	uint16_t __reserved2;
	char names[0]; // variable length
} GATE_PACKED;

struct gate_service_info_packet {
	struct gate_packet header;
	uint32_t __reserved;
	uint16_t count;
	uint16_t __reserved2;
	struct gate_service_info infos[0]; // variable length
} GATE_PACKED;

extern GATE_CONSTFUNC int __gate_get_abi_version(void) GATE_NOEXCEPT;
extern GATE_CONSTFUNC int32_t __gate_get_arg(void) GATE_NOEXCEPT;
extern GATE_CONSTFUNC size_t __gate_get_max_packet_size(void) GATE_NOEXCEPT;

#define gate_abi_version     (__gate_get_abi_version())
#define gate_arg             (__gate_get_arg())
#define gate_max_packet_size (__gate_get_max_packet_size())

extern void __gate_debug_write(const void *data, size_t size) GATE_NOEXCEPT;
extern GATE_CONSTFUNC void *__gate_func_ptr(enum gate_func_id id)
	GATE_NOEXCEPT;
extern GATE_NORETURN void __gate_exit(int status) GATE_NOEXCEPT;

void __gate_recv_packet(struct gate_packet *buf) GATE_NOEXCEPT;
size_t __gate_recv_packet_nonblock(struct gate_packet *buf) GATE_NOEXCEPT;
void __gate_send_packet(const struct gate_packet *data) GATE_NOEXCEPT;
size_t __gate_send_packet_nonblock(const struct gate_packet *data) GATE_NOEXCEPT;
GATE_PURE size_t __gate_nonblock_send_size() GATE_NOEXCEPT;

static inline void gate_debug(const char *s)
{
#ifdef NDEBUG
	(void) s; // attempt to suppress most warnings
#else
	size_t size = 0;

	for (const char *ptr = s; *ptr != '\0'; ptr++)
		size++;

	__gate_debug_write(s, size);
#endif
}

GATE_NORETURN
static inline void gate_exit(int status) GATE_NOEXCEPT
{
	__gate_exit(status);
}

static inline size_t gate_recv_packet(void *buf, size_t size, unsigned flags)
	GATE_NOEXCEPT
{
	if (size < gate_max_packet_size)
		__gate_exit(1);

	if (flags & GATE_RECV_FLAG_NONBLOCK) {
		return __gate_recv_packet_nonblock((struct gate_packet *) buf);
	} else {
		__gate_recv_packet((struct gate_packet *) buf);
		return ((const struct gate_packet *) buf)->size;
	}
}

static inline size_t gate_send_packet(const struct gate_packet *data, unsigned flags)
	GATE_NOEXCEPT
{
	if (data->__flags)
		__gate_exit(1);

	if (flags & GATE_SEND_FLAG_NONBLOCK) {
		return __gate_send_packet_nonblock(data);
	} else {
		__gate_send_packet(data);
		return data->size;
	}
}

GATE_PURE
static inline size_t gate_nonblock_send_size() GATE_NOEXCEPT
{
	return __gate_nonblock_send_size();
}

#ifdef __cplusplus
}
#endif

#endif
