// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#define __USE_EXTERN_INLINES // For CMSG_NXTHDR.

#include <signal.h>
#include <stddef.h>
#include <stdint.h>
#include <string.h>

#include <fcntl.h>
#include <sys/mman.h>
#include <sys/prctl.h>
#include <sys/resource.h>
#include <sys/socket.h>
#include <sys/syscall.h>
#include <sys/time.h>
#include <sys/types.h>

#include "align.h"
#include "debug.h"
#include "errors.h"
#include "runtime.h"
#include "syscall.h"

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
extern code gate_randomseed;
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

static int sys_close(int fd)
{
	return syscall1(SYS_close, fd);
}

static int sys_fcntl(int fd, int cmd, int arg)
{
	return syscall3(SYS_fcntl, fd, cmd, arg);
}

static void *sys_mmap(void *addr, size_t length, int prot, int flags, int fd, off_t offset)
{
	return (void *) syscall6(SYS_mmap, (uintptr_t) addr, length, prot, flags, fd, offset);
}

static int sys_mprotect(void *addr, size_t len, int prot)
{
	return syscall3(SYS_mprotect, (uintptr_t) addr, len, prot);
}

static int sys_prctl(int option, unsigned long arg2)
{
	return syscall2(SYS_prctl, option, arg2);
}

static ssize_t sys_recvmsg(int sockfd, struct msghdr *msg, int flags)
{
	return syscall3(SYS_recvmsg, sockfd, (uintptr_t) msg, flags);
}

static int sys_setrlimit(int resource, rlim_t limit)
{
	const struct rlimit buf = {
		.rlim_cur = limit,
		.rlim_max = limit,
	};

	return syscall2(SYS_setrlimit, resource, (uintptr_t) &buf);
}

// This is like imageInfo in runtime/process.go
struct image_info {
	uint16_t magic_number_1;
	uint8_t reserved;
	uint8_t init_routine;
	uint32_t page_size;
	uint64_t text_addr;
	uint64_t stack_addr;
	uint64_t heap_addr;
	uint64_t random_value;
	uint32_t text_size;
	uint32_t stack_size;
	uint32_t stack_unused;
	uint32_t globals_size;
	uint32_t init_memory_size;
	uint32_t grow_memory_size;
	uint32_t entry_addr;
	uint32_t magic_number_2;
} PACKED;

static int receive_info(struct image_info *buf, int *text_fd, int *state_fd)
{
	struct iovec iov = {
		.iov_base = buf,
		.iov_len = sizeof(struct image_info),
	};

	union {
		char buf[CMSG_SPACE(3 * sizeof(int))];
		struct cmsghdr alignment;
	} ctl;

	struct msghdr msg = {
		.msg_iov = &iov,
		.msg_iovlen = 1,
		.msg_control = ctl.buf,
		.msg_controllen = sizeof ctl.buf,
	};

	ssize_t n = sys_recvmsg(GATE_INPUT_FD, &msg, MSG_CMSG_CLOEXEC);
	if (n < 0)
		return -1;

	if (n != sizeof(struct image_info))
		return -1;

	if (msg.msg_flags & MSG_CTRUNC)
		return -1;

	struct cmsghdr *cmsg = CMSG_FIRSTHDR(&msg);
	if (cmsg == NULL)
		return -1;

	if (cmsg->cmsg_level != SOL_SOCKET)
		return -1;

	if (cmsg->cmsg_type != SCM_RIGHTS)
		return -1;

	const int *fds = (int *) CMSG_DATA(cmsg);
	int debug_flag;

	if (cmsg->cmsg_len == CMSG_LEN(2 * sizeof(int))) {
		debug_flag = 0;
		*text_fd = fds[0];
		*state_fd = fds[1];
	} else if (cmsg->cmsg_len == CMSG_LEN(3 * sizeof(int))) {
		if (fds[0] != GATE_DEBUG_FD)
			return -1;

		debug_flag = 1;
		*text_fd = fds[1];
		*state_fd = fds[2];
	} else {
		return -1;
	}

	if (CMSG_NXTHDR(&msg, cmsg))
		return -1;

	return debug_flag;
}

int main(void)
{
	if (GATE_SANDBOX)
		if (sys_prctl(PR_SET_DUMPABLE, 0) != 0)
			return ERR_LOAD_PRCTL_NOT_DUMPABLE;

	if (sys_prctl(PR_SET_TSC, PR_TSC_SIGSEGV) != 0)
		return ERR_LOAD_PRCTL_TSC_SIGSEGV;

	if (sys_setrlimit(RLIMIT_NOFILE, GATE_LIMIT_NOFILE) != 0)
		return ERR_LOAD_SETRLIMIT_NOFILE;

	if (sys_setrlimit(RLIMIT_NPROC, 0) != 0)
		return ERR_LOAD_SETRLIMIT_NPROC;

	// Image info and file descriptors

	struct image_info info = {0};
	int text_fd = -1;
	int state_fd = -1;
	int debug_flag = receive_info(&info, &text_fd, &state_fd);
	if (debug_flag < 0)
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

	code *debug_func = debug_flag ? &gate_debug : &gate_nop;

	// These assignments reflect the moduleFunctions map in runtime/abi/abi.go
	*(vector_end - 8) = info.random_value;
	*(vector_end - 7) = runtime_func_addr(runtime_ptr, debug_func);
	*(vector_end - 6) = runtime_func_addr(runtime_ptr, &gate_exit);
	*(vector_end - 5) = runtime_func_addr(runtime_ptr, &gate_io);
	*(vector_end - 4) = runtime_func_addr(runtime_ptr, &gate_randomseed);
	*(vector_end - 3) = runtime_func_addr(runtime_ptr, &current_memory);
	*(vector_end - 2) = runtime_func_addr(runtime_ptr, &grow_memory);
	*(vector_end - 1) = runtime_func_addr(runtime_ptr, &trap_handler);

	if (sys_mprotect(runtime_ptr, info.page_size, PROT_READ | PROT_EXEC) != 0)
		return ERR_LOAD_MPROTECT_VECTOR;

	// Text

	void *text_ptr = sys_mmap((void *) info.text_addr, info.text_size, PROT_READ | PROT_EXEC, MAP_PRIVATE | MAP_FIXED | MAP_NORESERVE, text_fd, 0);
	if (text_ptr != (void *) info.text_addr)
		return ERR_LOAD_MMAP_TEXT;

	if (sys_close(text_fd) != 0)
		return ERR_LOAD_CLOSE_TEXT;

	// Stack

	void *stack_buf = sys_mmap((void *) info.stack_addr, info.stack_size, PROT_READ | PROT_WRITE, MAP_SHARED | MAP_FIXED | MAP_NORESERVE, state_fd, 0);
	if (stack_buf != (void *) info.stack_addr)
		return ERR_LOAD_MMAP_STACK;

	*(uint32_t *) stack_buf = info.init_memory_size >> 16; // WebAssembly pages.

	void *stack_limit = stack_buf + GATE_STACK_LIMIT_OFFSET;
	uint64_t *stack_ptr = stack_buf + info.stack_unused;

	if (info.stack_unused == info.stack_size) {
		// Synthesize initial stack frame for start or entry routine
		// (checked in runtime/process.go).
		*--stack_ptr = info.entry_addr;
	}

	// Globals and memory

	size_t heap_offset = (size_t) info.stack_size;
	size_t heap_size = (size_t) info.globals_size + (size_t) info.grow_memory_size;

	void *memory_ptr = NULL;

	if (heap_size > 0) {
		void *heap_ptr = sys_mmap((void *) info.heap_addr, heap_size, PROT_NONE, MAP_SHARED | MAP_FIXED | MAP_NORESERVE, state_fd, heap_offset);
		if (heap_ptr != (void *) info.heap_addr)
			return ERR_LOAD_MMAP_HEAP;

		size_t allocated = info.globals_size + info.init_memory_size;
		if (allocated > 0 && sys_mprotect(heap_ptr, allocated, PROT_READ | PROT_WRITE) != 0)
			return ERR_LOAD_MPROTECT_HEAP;

		memory_ptr = heap_ptr + info.globals_size;
	}

	if (sys_close(state_fd) != 0)
		return ERR_LOAD_CLOSE_STATE;

	// Enable I/O signals for sending file descriptor.

	if (sys_fcntl(GATE_OUTPUT_FD, F_SETFL, O_ASYNC | O_NONBLOCK) != 0)
		return ERR_LOAD_FCNTL_OUTPUT;

	// Start runtime.

	code *init_routine = GATE_SANDBOX ? &runtime_init : &runtime_init_no_sandbox;

	register void *rax asm("rax") = stack_ptr;
	register void *rbx asm("rbx") = stack_limit;
	register uint64_t rbp asm("rbp") = runtime_func_addr(runtime_ptr, init_routine);
	register uint64_t rsi asm("rsi") = align_size(GATE_LOADER_STACK_SIZE, info.page_size);
	register uint64_t r9 asm("r9") = runtime_func_addr(runtime_ptr, &signal_handler);
	register uint64_t r10 asm("r10") = runtime_func_addr(runtime_ptr, &signal_restorer);
	register uint64_t r11 asm("r11") = info.page_size - 1;
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
