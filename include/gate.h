#ifndef _GATE_H
#define _GATE_H

#include <stddef.h>
#include <stdint.h>

#include <gate/args.h>

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

enum gate_op_code {
	GATE_OP_CODE_NONE    = 0,
	GATE_OP_CODE_ORIGIN  = 1,
};

#define GATE_OP_FLAG_POLLOUT 0x1

struct gate_op {
	uint32_t size;
	uint16_t code;
	uint16_t flags;
} GATE_PACKED;

enum gate_ev_code {
	GATE_EV_CODE_POLLOUT = 1,
};

struct gate_ev {
	uint32_t size;
	uint16_t code;
	uint16_t reserved;
} GATE_PACKED;

extern const uint32_t __gate_args[];

#define gate_heap_size  ((size_t) __gate_args[GATE_ARG_HEAP_SIZE])
#define gate_heap_addr  ((uintptr_t) __gate_args[GATE_ARG_HEAP_ADDR])
#define gate_op_maxsize ((size_t) __gate_args[GATE_ARG_OP_MAXSIZE])

GATE_NORETURN
static inline void gate_exit(int status) GATE_NOEXCEPT
{
	GATE_NORETURN void (*func)(int) GATE_NOEXCEPT = (GATE_NORETURN void (*)(int) GATE_NOEXCEPT) (uintptr_t) __gate_args[GATE_ARG_FUNC_EXIT];
	func(status);
}

static inline size_t gate_recv(long reserved, void *buf, size_t size) GATE_NOEXCEPT
{
	size_t (*func)(long, void *, size_t) GATE_NOEXCEPT = (size_t (*)(long, void *, size_t) GATE_NOEXCEPT) (uintptr_t) __gate_args[GATE_ARG_FUNC_RECV];
	return func(reserved, buf, size);
}

static inline size_t gate_send(long reserved, const void *data, size_t size) GATE_NOEXCEPT
{
	size_t (*func)(long, const void *, size_t) GATE_NOEXCEPT = (size_t (*)(long, const void *, size_t) GATE_NOEXCEPT) (uintptr_t) __gate_args[GATE_ARG_FUNC_SEND];
	return func(reserved, data, size);
}

static inline void gate_recv_full(void *buf, size_t size) GATE_NOEXCEPT
{
	for (size_t pos = 0; pos < size; )
		pos += gate_recv(0, (uint8_t *) buf + pos, size - pos);
}

static inline void gate_send_full(const void *data, size_t size) GATE_NOEXCEPT
{
	for (size_t pos = 0; pos < size; )
		pos += gate_send(0, (const uint8_t *) data + pos, size - pos);
}

#ifdef __cplusplus
}
#endif

#endif
