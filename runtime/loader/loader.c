// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <signal.h>
#include <stddef.h>
#include <stdint.h>
#include <string.h>

#include <fcntl.h>
#include <sys/mman.h>
#include <sys/prctl.h>
#include <sys/syscall.h>
#include <sys/types.h>

#include "debug.h"
#include "errors.h"
#include "runtime.h"

#define NORETURN __attribute__((noreturn))
#define PACKED __attribute__((packed))

#define SYS_SA_RESTORER 0x04000000
#define SIGACTION_FLAGS (SA_RESTART | SYS_SA_RESTORER | SA_SIGINFO)

// Avoiding function prototypes avoids GOT section.
typedef const struct {
	char dummy;
} code;

extern code current_memory;
extern code gate_debug;
extern code gate_exit;
extern code gate_io;
extern code gate_nop;
extern code grow_memory;
extern code retpoline;
extern code runtime_code_begin;
extern code runtime_code_end;
extern code runtime_init;
extern code runtime_init_no_sandbox;
extern code signal_handler;
extern code signal_restorer;
extern code sys_exit;
extern code trap_handler;

static uintptr_t runtime_func_addr(const void *new_base, code *func_ptr)
{
	return (uintptr_t) new_base + ((uintptr_t) func_ptr - GATE_LOADER_ADDR);
}

static int sys_personality(unsigned long persona)
{
	int retval;

	asm volatile(
		"syscall"
		: "=a"(retval)
		: "a"(SYS_personality), "D"(persona)
		: "cc", "rcx", "r11", "memory");

	return retval;
}

static int sys_prctl(int option, unsigned long arg2)
{
	int retval;

	asm volatile(
		"syscall"
		: "=a"(retval)
		: "a"(SYS_prctl), "D"(option), "S"(arg2)
		: "cc", "rcx", "r11", "memory");

	return retval;
}

static int sys_fcntl(int fd, int cmd, int arg)
{
	int retval;

	asm volatile(
		"syscall"
		: "=a"(retval)
		: "a"(SYS_fcntl), "D"(fd), "S"(cmd), "d"(arg)
		: "cc", "rcx", "r11", "memory");

	return retval;
}

static ssize_t sys_read(int fd, void *buf, size_t count)
{
	ssize_t retval;

	asm volatile(
		"syscall"
		: "=a"(retval)
		: "a"(SYS_read), "D"(fd), "S"(buf), "d"(count)
		: "cc", "rcx", "r11", "memory");

	return retval;
}

static void *sys_mmap(void *addr, size_t length, int prot, int flags, int fd, off_t offset)
{
	void *retval;

	register void *rdi asm("rdi") = addr;
	register size_t rsi asm("rsi") = length;
	register int rdx asm("rdx") = prot;
	register int r10 asm("r10") = flags;
	register int r8 asm("r8") = fd;
	register off_t r9 asm("r9") = offset;

	asm volatile(
		"syscall"
		: "=a"(retval)
		: "a"(SYS_mmap), "r"(rdi), "r"(rsi), "r"(rdx), "r"(r10), "r"(r8), "r"(r9)
		: "cc", "rcx", "r11", "memory");

	return retval;
}

static int sys_mprotect(void *addr, size_t len, int prot)
{
	int retval;

	asm volatile(
		"syscall"
		: "=a"(retval)
		: "a"(SYS_mprotect), "D"(addr), "S"(len), "d"(prot)
		: "cc", "rcx", "r11", "memory");

	return retval;
}

static int sys_close(int fd)
{
	int retval;

	asm volatile(
		"syscall"
		: "=a"(retval)
		: "a"(SYS_close), "D"(fd)
		: "cc", "rcx", "r11", "memory");

	return retval;
}

static int read_full(void *buf, size_t size)
{
	for (size_t pos = 0; pos < size;) {
		ssize_t len = sys_read(0, buf + pos, size - pos);
		if (len < 0)
			return -1;
		pos += len;
	}

	return 0;
}

int main(void)
{
	if (GATE_SANDBOX) {
		if (sys_prctl(PR_SET_DUMPABLE, 0) != 0)
			return ERR_LOAD_PRCTL_NOT_DUMPABLE;
	}

	// Undo the personality change by executor.
	if (sys_personality(0) < 0)
		return ERR_LOAD_PERSONALITY_DEFAULT;

	// This is like imageInfo in runtime/process.go
	struct PACKED {
		uint32_t magic_number_1;
		uint32_t page_size;
		uint64_t text_addr;
		uint64_t stack_addr;
		uint64_t heap_addr;
		uint32_t text_size;
		uint32_t stack_size;
		uint32_t stack_unused;
		uint32_t globals_size;
		uint32_t init_memory_size;
		uint32_t grow_memory_size;
		uint16_t init_routine;
		uint16_t debug_flag;
		uint32_t magic_number_2;
	} info;

	if (read_full(&info, sizeof info) != 0)
		return ERR_LOAD_READ_INFO;

	if (info.magic_number_1 != GATE_MAGIC_NUMBER_1)
		return ERR_LOAD_MAGIC_1;

	if (info.magic_number_2 != GATE_MAGIC_NUMBER_2)
		return ERR_LOAD_MAGIC_2;

	// Runtime: code at start, import vector at end

	uint64_t runtime_addr = info.text_addr - (uint64_t) info.page_size;

	void *runtime_ptr = sys_mmap((void *) runtime_addr, info.page_size, PROT_READ | PROT_WRITE, MAP_PRIVATE | MAP_ANONYMOUS | MAP_FIXED, -1, 0);
	if (runtime_ptr != (void *) runtime_addr)
		return ERR_LOAD_MMAP_VECTOR;

	uintptr_t runtime_size = (uintptr_t) &runtime_code_end - (uintptr_t) &runtime_code_begin;
	memcpy(runtime_ptr + ((uintptr_t) &runtime_code_begin - GATE_LOADER_ADDR), &runtime_code_begin, runtime_size);

	uint64_t *vector_end = (uint64_t *) (runtime_ptr + info.page_size);

	code *debug_func = (info.debug_flag & 1) ? &gate_debug : &gate_nop;

	// These assignments reflect the moduleFunctions map in runtime/abi/abi.go
	*(vector_end - 6) = runtime_func_addr(runtime_ptr, debug_func);
	*(vector_end - 5) = runtime_func_addr(runtime_ptr, &gate_exit);
	*(vector_end - 4) = runtime_func_addr(runtime_ptr, &gate_io);
	*(vector_end - 3) = runtime_func_addr(runtime_ptr, &current_memory);
	*(vector_end - 2) = runtime_func_addr(runtime_ptr, &grow_memory);
	*(vector_end - 1) = runtime_func_addr(runtime_ptr, &trap_handler);

	if (sys_mprotect(runtime_ptr, info.page_size, PROT_READ | PROT_EXEC) != 0)
		return ERR_LOAD_MPROTECT_VECTOR;

	// Text

	void *text_ptr = sys_mmap((void *) info.text_addr, info.text_size, PROT_READ | PROT_EXEC, MAP_PRIVATE | MAP_FIXED | MAP_NORESERVE, GATE_IMAGE_FD, 0);
	if (text_ptr != (void *) info.text_addr)
		return ERR_LOAD_MMAP_TEXT;

	// Stack

	size_t stack_offset = info.text_size;

	void *stack_buf = sys_mmap((void *) info.stack_addr, info.stack_size, PROT_READ | PROT_WRITE, MAP_SHARED | MAP_FIXED | MAP_NORESERVE, GATE_IMAGE_FD, stack_offset);
	if (stack_buf != (void *) info.stack_addr)
		return ERR_LOAD_MMAP_STACK;

	*(uint32_t *) stack_buf = info.init_memory_size >> 16;

	void *stack_limit = stack_buf + GATE_STACK_LIMIT_OFFSET;
	void *stack_ptr = stack_buf + info.stack_unused;

	// Globals and memory

	size_t heap_offset = stack_offset + (size_t) info.stack_size;
	size_t heap_size = (size_t) info.globals_size + (size_t) info.grow_memory_size;

	void *memory_ptr = NULL;

	if (heap_size > 0) {
		void *heap_ptr = sys_mmap((void *) info.heap_addr, heap_size, PROT_NONE, MAP_SHARED | MAP_FIXED | MAP_NORESERVE, GATE_IMAGE_FD, heap_offset);
		if (heap_ptr != (void *) info.heap_addr)
			return ERR_LOAD_MMAP_HEAP;

		size_t allocated = info.globals_size + info.init_memory_size;
		if (allocated > 0 && sys_mprotect(heap_ptr, allocated, PROT_READ | PROT_WRITE) != 0)
			return ERR_LOAD_MPROTECT_HEAP;

		memory_ptr = heap_ptr + info.globals_size;
	}

	// Mappings done.

	if (sys_close(GATE_IMAGE_FD) != 0)
		return ERR_LOAD_CLOSE_IMAGE;

	// Enable I/O signals for sending file descriptor.

	if (sys_fcntl(GATE_OUTPUT_FD, F_SETFL, O_ASYNC | O_NONBLOCK) != 0)
		return ERR_LOAD_FCNTL_OUTPUT;

	// Start runtime.

	code *init_routine = GATE_SANDBOX ? &runtime_init : &runtime_init_no_sandbox;

	size_t pagemask = info.page_size - 1;

	register void *rax asm("rax") = stack_ptr;
	register void *rbx asm("rbx") = stack_limit;
	register uint64_t rbp asm("rbp") = runtime_func_addr(runtime_ptr, init_routine);
	register uint64_t rsi asm("rsi") = (GATE_LOADER_STACK_SIZE + pagemask) & ~pagemask;
	register uint64_t r9 asm("r9") = runtime_func_addr(runtime_ptr, &signal_handler);
	register uint64_t r10 asm("r10") = runtime_func_addr(runtime_ptr, &signal_restorer);
	register uint64_t r11 asm("r11") = pagemask;
	register void *r14 asm("r14") = memory_ptr;
	register void *r15 asm("r15") = text_ptr + (uintptr_t) info.init_routine;

	// clang-format off

	asm volatile(
		// Replace stack.

		"mov  %%rax, %%rsp                          \n"

		// Load the stack ptr saved in _start, and unmap old stack
		// (ASLR breaks this).

		"movq %%mm7, %%rdi                          \n" // addr = stack top
		"add  %%r11, %%rdi                          \n" // addr += pagemask
		"not  %%r11                                 \n" // ~pagemask
		"and  %%r11, %%rdi                          \n" // addr &= ~pagemask
		"sub  %%rsi, %%rdi                          \n" // addr -= stack size
		"mov  $"xstr(SYS_munmap)", %%eax            \n"
		"syscall                                    \n"
		"mov  $"xstr(ERR_LOAD_MUNMAP_STACK)", %%edi \n"
		"test %%rax, %%rax                          \n"
		"jne  sys_exit                              \n"

		// Build sigaction structure on stack.  Using 32 bytes of red
		// zone.

		"sub  $32, %%rsp                            \n" // sizeof (struct sigaction)

		"mov  %%rsp, %%rsi                          \n" // sigaction act
		"mov  %%r9, 0(%%rsi)                        \n" // sa_handler
		"movq $"xstr(SIGACTION_FLAGS)", 8(%%rsi)    \n" // sa_flags
		"mov  %%r10, 16(%%rsi)                      \n" // sa_restorer
		"movq $0, 24(%%rsi)                         \n" // sa_mask

		"xor  %%edx, %%edx                          \n" // sigaction oldact
		"mov  $8, %%r10d                            \n" // sigaction mask size

		// Async I/O signal handler.

		"mov  $"xstr(SIGIO)", %%edi                 \n" // sigaction signum
		"mov  $"xstr(SYS_rt_sigaction)", %%eax      \n"
		"syscall                                    \n"
		"mov  $"xstr(ERR_LOAD_SIGACTION)", %%edi    \n"
		"test %%rax, %%rax                          \n"
		"jne  sys_exit                              \n"

		// Segmentation fault signal handler.

		"mov  $"xstr(SIGSEGV)", %%edi               \n" // sigaction signum
		"mov  $"xstr(SYS_rt_sigaction)", %%eax      \n"
		"syscall                                    \n"
		"mov  $"xstr(ERR_LOAD_SIGACTION)", %%edi    \n"
		"test %%rax, %%rax                          \n"
		"jne  sys_exit                              \n"

		// Suspend signal handler.

		"mov  $"xstr(SIGXCPU)", %%edi               \n" // siaction signum
		"mov  $"xstr(SYS_rt_sigaction)", %%eax      \n"
		"syscall                                    \n"
		"mov  $"xstr(ERR_LOAD_SIGACTION)", %%edi    \n"
		"test %%rax, %%rax                          \n"
		"jne  sys_exit                              \n"

		"add  $32, %%rsp                            \n" // sizeof (struct sigaction)

		// Execute runtime_init.

		"mov  %%rbp, %%rcx                          \n"
		"jmp  retpoline                             \n"
		:
		: "r"(rax), "r"(rbx), "r"(rbp), "r"(rsi), "r"(r9), "r"(r10), "r"(r11), "r"(r14), "r"(r15));

	// clang-format on

	__builtin_unreachable();
}
