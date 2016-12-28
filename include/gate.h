#ifndef _GATE_H
#define _GATE_H

#include <stddef.h>
#include <stdint.h>

#ifndef GATE_CONSTFUNC
# define GATE_CONSTFUNC __attribute__ ((const))
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

enum gate_func_id {
	__GATE_FUNC_RESERVED
};

#define GATE_RECV_FLAG_NONBLOCK 0x1

enum gate_op_code {
	GATE_OP_CODE_NONE   = 0,
	GATE_OP_CODE_ORIGIN = 1,
};

#define GATE_OP_FLAG_POLLOUT 0x1

struct gate_op_header {
	uint32_t size;
	uint16_t code;
	uint16_t flags;
} GATE_PACKED;

enum gate_ev_code {
	GATE_EV_CODE_POLLOUT = 0,
	GATE_EV_CODE_ORIGIN  = 1,
};

struct gate_ev_header {
	uint32_t size;
	uint16_t code;
	uint16_t __reserved;
} GATE_PACKED;

// extern const int __gate_abi_version;
// extern const size_t __gate_max_packet_size;

extern GATE_CONSTFUNC int __gate_get_abi_version(void) GATE_NOEXCEPT;
extern GATE_CONSTFUNC size_t __gate_get_max_packet_size(void) GATE_NOEXCEPT;

#define gate_abi_version     (__gate_get_abi_version())
#define gate_max_packet_size (__gate_get_max_packet_size())

extern void __gate_debug_write(const void *data, size_t size) GATE_NOEXCEPT;
extern GATE_CONSTFUNC void *__gate_func_ptr(enum gate_func_id id) GATE_NOEXCEPT;
extern GATE_NORETURN void __gate_exit(int status) GATE_NOEXCEPT;
extern size_t __gate_recv(void *buf, size_t size, unsigned int flags) GATE_NOEXCEPT;
extern void __gate_send(const void *data, size_t size) GATE_NOEXCEPT;

#ifdef NDEBUG
# define gate_debug(s)
#else
static inline void gate_debug(const char *s)
{
	size_t size = 0;

	for (const char *ptr = s; *ptr != '\0'; ptr++)
		size++;

	__gate_debug_write(s, size);
}
#endif

GATE_NORETURN
static inline void gate_exit(int status) GATE_NOEXCEPT
{
	__gate_exit(status);
}

static inline size_t gate_recv_packet(void *buf, size_t size, unsigned int flags) GATE_NOEXCEPT
{
	if (size < gate_max_packet_size)
		gate_exit(1);

	unsigned int other_flags = flags & ~(unsigned int) GATE_RECV_FLAG_NONBLOCK;

	if ((flags & GATE_RECV_FLAG_NONBLOCK) != 0) {
		size_t remain = __gate_recv(buf, sizeof (struct gate_ev_header), flags);
		if (remain == sizeof (struct gate_ev_header))
			return 0;

		if (remain > 0) {
			size_t n = sizeof (struct gate_ev_header) - remain;
			__gate_recv((char *) buf + n, remain, other_flags);
		}
	} else {
		__gate_recv(buf, sizeof (struct gate_ev_header), other_flags);
	}

	const struct gate_ev_header *header = (struct gate_ev_header *) buf;

	__gate_recv((char *) buf + sizeof (struct gate_ev_header), header->size - sizeof (struct gate_ev_header), other_flags);

	return header->size;
}

static inline void gate_send_packet(const struct gate_op_header *packet) GATE_NOEXCEPT
{
	__gate_send(packet, packet->size);
}

#ifdef __cplusplus
}
#endif

#endif
