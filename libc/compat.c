#include <stddef.h>
#include <stdint.h>

#include <gate.h>

#define __NR_brk 45

#define FILE void

typedef int64_t off_t;
typedef unsigned long pthread_t;

int __wasm_current_memory(void);
void __wasm_grow_memory(int addr);

GATE_NORETURN
static void fail(char code)
{
	static char buf[sizeof (struct gate_op_packet) + 4];

	struct gate_op_packet *head = (struct gate_op_packet *) buf;
	head->size = sizeof (buf);
	head->code = GATE_OP_CODE_ORIGIN;

	buf[sizeof (struct gate_op_packet) + 0] = '\n';
	buf[sizeof (struct gate_op_packet) + 1] = code;
	buf[sizeof (struct gate_op_packet) + 2] = '\n';
	buf[sizeof (struct gate_op_packet) + 3] = '\n';

	gate_send_packet(head);
	gate_exit(1);
}

static int sys_brk(int addr)
{
	static int wasm_pages;
	static int brk_addr;

	if (wasm_pages == 0) {
		wasm_pages = __wasm_current_memory();
		brk_addr = wasm_pages << 16;
	}

	if (addr > 0) {
		int increment = addr - (wasm_pages << 16);

		if (increment <= 0) {
			// TODO: fill freed memory with zero
			brk_addr = addr;
		} else {
			int increment_pages = (increment + 0xffff) >> 16;

			__wasm_grow_memory(increment_pages);

			int n = wasm_pages + increment_pages;

			if (__wasm_current_memory() == n) {
				wasm_pages = n;
				brk_addr = addr;
			}
		}
	}

	return brk_addr;
}

void abort(void)
{
	fail('A');
}

pthread_t pthread_self(void)
{
	fail('B');
}

void _Exit(int code)
{
	gate_exit(code);
}

int __madvise(void *addr, size_t len, int advice)
{
	fail('C');
}

void *__mmap(void *start, size_t len, int prot, int flags, int fd, off_t off)
{
	fail('D');
}

void *__mremap(void *old_addr, size_t old_len, size_t new_len, int flags, void *new_addr)
{
	fail('E');
}

int __munmap(void *start, size_t len)
{
	fail('F');
}

size_t __stdio_write(FILE *ptr, const unsigned char *buf, size_t count)
{
	fail('G');
}

size_t __stdout_write(FILE *ptr, const unsigned char *buf, size_t count)
{
	fail('H');
}

long __syscall0(long nr)
{
	fail('I');
}

long __syscall1(long nr, long a1)
{
	switch (nr) {
	case __NR_brk:
		return sys_brk(a1);

	default:
		fail('J');
	}
}

long __syscall3(long nr, long a1, long a2, long a3)
{
	fail('K');
}

long __syscall_ret(long nr)
{
	fail('L');
}
