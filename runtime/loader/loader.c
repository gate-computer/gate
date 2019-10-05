// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#define __USE_EXTERN_INLINES // For CMSG_NXTHDR.

#include <signal.h>
#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>
#include <string.h>
#include <time.h>

#include <elf.h>
#include <fcntl.h>
#include <sys/mman.h>
#include <sys/prctl.h>
#include <sys/resource.h>
#include <sys/socket.h>
#include <sys/syscall.h>
#include <sys/time.h>
#include <sys/types.h>

#include "align.h"
#include "attribute.h"
#include "debug.h"
#include "errors.h"
#include "runtime.h"
#include "syscall.h"

#define SYS_SA_RESTORER 0x04000000
#define SIGACTION_FLAGS (SYS_SA_RESTORER | SA_SIGINFO)

#ifdef __ANDROID__
#define ANDROID 1
#define MAYBE_MAP_FIXED 0
#else
#define ANDROID 0
#define MAYBE_MAP_FIXED MAP_FIXED
#endif

#include "loader.h"

#if defined(__clang__) || defined(__aarch64__)
void *memcpy(void *dest, const void *src, size_t n)
{
	for (size_t i = 0; i < n; i++)
		((uint8_t *) dest)[i] = ((const uint8_t *) src)[i];
	return dest;
}

size_t strlen(const char *s)
{
	size_t n = 0;
	while (*s++)
		n++;
	return n;
}
#endif

#if defined(__clang__)
struct cmsghdr *__cmsg_nxthdr(struct msghdr *msg, struct cmsghdr *cmsg)
{
	struct cmsghdr *ptr = (void *) cmsg + CMSG_ALIGN(cmsg->cmsg_len);
	size_t len = (void *) ptr + 1 - (void *) msg->msg_control;
	if (len > msg->msg_controllen)
		return NULL;
	return ptr;
}
#endif // __clang__

// Avoiding function prototypes avoids GOT section.
typedef const struct {
	char dummy;
} code;

extern code current_memory;
extern code grow_memory;
extern code retpoline;
extern code rt_debug;
extern code rt_nop;
extern code rt_poll;
extern code rt_random;
extern code rt_read;
extern code rt_write;
extern code rt_stop;
extern code rt_time;
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
	intptr_t ret = syscall6(SYS_mmap, (uintptr_t) addr, length, prot, flags, fd, offset);
	if (ret < 0 && ret > -4096)
		return MAP_FAILED;
	return (void *) ret;
}

static int sys_mprotect(void *addr, size_t len, int prot)
{
	return syscall3(SYS_mprotect, (uintptr_t) addr, len, prot);
}

static void *sys_mremap(void *old_addr, size_t old_size, size_t new_size, int flags)
{
	intptr_t ret = syscall4(SYS_mremap, (uintptr_t) old_addr, old_size, new_size, flags);
	if (ret < 0 && ret > -4096)
		return MAP_FAILED;
	return (void *) ret;
}

static int sys_personality(unsigned long persona)
{
	return syscall1(SYS_personality, persona);
}

static int sys_prctl(int option, unsigned long arg2)
{
	return syscall2(SYS_prctl, option, arg2);
}

static ssize_t sys_read(int fd, void *buf, size_t count)
{
	return syscall3(SYS_read, fd, (uintptr_t) buf, count);
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
	uint32_t magic_number_1;
	uint32_t page_size;
	uint64_t text_addr;
	uint64_t stack_addr;
	uint64_t heap_addr;
	uint64_t random[2];
	uint32_t text_size;
	uint32_t stack_size;
	uint32_t stack_unused;
	uint32_t globals_size;
	uint32_t init_memory_size;
	uint32_t grow_memory_size;
	uint32_t init_routine;
	uint32_t entry_addr;
	uint64_t monotonic_time;
	uint32_t time_mask;
	uint32_t magic_number_2;
} PACKED;

// This is like stackVars in image/instance.go
struct stack_vars {
	uint32_t stack_unused;
	uint32_t current_memory_pages; // WebAssembly pages.
	uint64_t monotonic_time_snapshot;
	int32_t random_avail;
	uint32_t reserved;
	uint64_t text_addr;
	uint64_t magic[4];
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

static const Elf64_Shdr *elf_section(const Elf64_Ehdr *elf, unsigned index)
{
	return (const void *) elf + elf->e_shoff + elf->e_shentsize * index;
}

static const char *elf_string(const Elf64_Ehdr *elf, unsigned strtab_index, unsigned str_index)
{
	const Elf64_Shdr *strtab = (const void *) elf + elf->e_shoff + elf->e_shentsize * strtab_index;
	return (void *) elf + strtab->sh_offset + str_index;
}

int main(int argc, char **argv, char **envp)
{
	// _start routine smuggles vDSO ELF address as argv pointer.
	// Use it to lookup clock_gettime function.

	const Elf64_Ehdr *vdso = (void *) argv;
	uintptr_t clock_gettime_addr = 0;

	for (unsigned i = 0; i < vdso->e_shnum; i++) {
		const Elf64_Shdr *shdr = elf_section(vdso, i);
		if (shdr->sh_type != SHT_DYNSYM)
			continue;

		for (uint64_t off = 0; off < shdr->sh_size; off += shdr->sh_entsize) {
			const Elf64_Sym *sym = (const void *) vdso + shdr->sh_offset + off;

			const char *name = elf_string(vdso, shdr->sh_link, sym->st_name);
			if (!strcmp_clock_gettime(name))
				continue;

			clock_gettime_addr = (uintptr_t) vdso + sym->st_value;
			goto clock_gettime_found;
		}
	}
	return ERR_LOAD_NO_CLOCK_GETTIME;

clock_gettime_found:
	// Miscellaneous preparations.

	if (GATE_SANDBOX) {
		if (sys_prctl(PR_SET_DUMPABLE, 0) != 0)
			return ERR_LOAD_PRCTL_NOT_DUMPABLE;
	}

	if (MAYBE_MAP_FIXED == 0) {
		// Undo the ADDR_NO_RANDOMIZE setting as manually randomized
		// addresses might not be used.
		if (sys_personality(0) < 0)
			return ERR_LOAD_PERSONALITY_DEFAULT;
	}

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

	// Time

	struct timespec t;

	int (*gettime)(clockid_t, struct timespec *) = (void *) clock_gettime_addr;
	if (gettime(CLOCK_MONOTONIC_COARSE, &t) != 0)
		return ERR_LOAD_CLOCK_GETTIME;

	t.tv_nsec &= info.time_mask;
	uint64_t local_monotonic_time_base = (uint64_t) t.tv_sec * 1000000000ULL + (uint64_t) t.tv_nsec;

	// Runtime: code at start, import vector at end (and maybe space for text)

	uint64_t runtime_addr = info.text_addr - (uint64_t) info.page_size;
	size_t runtime_map_size = info.page_size + (ANDROID ? info.text_size : 0);

	void *runtime_ptr = sys_mmap((void *) runtime_addr, runtime_map_size, PROT_READ | PROT_WRITE, MAP_PRIVATE | MAP_ANONYMOUS | MAYBE_MAP_FIXED, -1, 0);
	if (runtime_ptr == MAP_FAILED)
		return ERR_LOAD_MMAP_VECTOR;

	uintptr_t runtime_size = (uintptr_t) &runtime_code_end - (uintptr_t) &runtime_code_begin;
	memcpy(runtime_ptr + ((uintptr_t) &runtime_code_begin - GATE_LOADER_ADDR), &runtime_code_begin, runtime_size);

	// TODO: check that runtime and vector contents don't overlap

	uint64_t *vector_end = (uint64_t *) (runtime_ptr + info.page_size);

	code *debug_func = debug_flag ? &rt_debug : &rt_nop;

	// These assignments reflect the rtFunctions map in runtime/abi/abi.go
	*(vector_end - 17) = runtime_func_addr(runtime_ptr, &rt_stop);
	*(vector_end - 16) = runtime_func_addr(runtime_ptr, debug_func);
	*(vector_end - 15) = runtime_func_addr(runtime_ptr, &rt_write);
	*(vector_end - 14) = runtime_func_addr(runtime_ptr, &rt_read);
	*(vector_end - 13) = runtime_func_addr(runtime_ptr, &rt_poll);
	*(vector_end - 12) = runtime_func_addr(runtime_ptr, &rt_time);
	*(vector_end - 11) = clock_gettime_addr;
	*(vector_end - 10) = local_monotonic_time_base;
	*(vector_end - 9) = info.time_mask;
	*(vector_end - 8) = info.random[0];
	*(vector_end - 7) = info.random[1];
	*(vector_end - 6) = runtime_func_addr(runtime_ptr, &rt_random);
	*(vector_end - 5) = runtime_func_addr(runtime_ptr, &rt_nop);
	*(vector_end - 4) = info.grow_memory_size >> 16;
	*(vector_end - 3) = runtime_func_addr(runtime_ptr, &current_memory);
	*(vector_end - 2) = runtime_func_addr(runtime_ptr, &grow_memory);
	*(vector_end - 1) = runtime_func_addr(runtime_ptr, &trap_handler);

	// Text

	void *text_ptr = vector_end;

	if (ANDROID) {
		if (sys_read(text_fd, text_ptr, info.text_size) != info.text_size)
			return ERR_LOAD_READ_TEXT;
	} else {
		void *p = sys_mmap(text_ptr, info.text_size, PROT_READ | PROT_EXEC, MAP_PRIVATE | MAP_FIXED, text_fd, 0);
		if (p == MAP_FAILED)
			return ERR_LOAD_MMAP_TEXT;
	}

	if (sys_close(text_fd) != 0)
		return ERR_LOAD_CLOSE_TEXT;

	// Runtime (and maybe text)

	if (sys_mprotect(runtime_ptr, runtime_map_size, PROT_READ | PROT_EXEC) != 0)
		return ERR_LOAD_MPROTECT_VECTOR;

	// Stack

	void *stack_buf = sys_mmap((void *) info.stack_addr, info.stack_size, PROT_READ | PROT_WRITE, MAP_SHARED | MAYBE_MAP_FIXED, state_fd, 0);
	if (stack_buf == MAP_FAILED)
		return ERR_LOAD_MMAP_STACK;

	volatile struct stack_vars *vars = stack_buf;
	vars->stack_unused = 0; // Invalidate state (in case of re-entry).
	vars->current_memory_pages = info.init_memory_size >> 16;
	vars->monotonic_time_snapshot = info.monotonic_time;
	vars->random_avail = sizeof info.random;
	vars->reserved = 0;
	vars->text_addr = (uint64_t) text_ptr;
	for (unsigned i = 0; i < sizeof vars->magic / sizeof vars->magic[0]; i++)
		vars->magic[i] = GATE_STACK_MAGIC;

	void *stack_limit = stack_buf + GATE_STACK_LIMIT_OFFSET;
	uint64_t *stack_ptr = stack_buf + info.stack_unused;

	if (info.stack_unused == info.stack_size) {
		// Synthesize initial stack frame for start or entry routine
		// (checked in runtime/process.go).
		*--stack_ptr = info.entry_addr;
	}

	// Globals and memory

	size_t heap_offset = (size_t) info.stack_size;
	size_t heap_allocated = (size_t) info.globals_size + (size_t) info.init_memory_size;
	size_t heap_size = (size_t) info.globals_size + (size_t) info.grow_memory_size;
	void *heap_ptr;

	if (ANDROID) {
		size_t space = (size_t) info.globals_size + MEMORY_ADDRESS_RANGE;

		heap_ptr = sys_mmap((void *) info.heap_addr, space, PROT_READ | PROT_WRITE, MAP_SHARED, state_fd, heap_offset);
		if (heap_ptr == MAP_FAILED)
			return ERR_LOAD_MMAP_HEAP;

		void *ret = sys_mremap(heap_ptr, space, heap_allocated, 0);
		if (ret == MAP_FAILED || ret != heap_ptr)
			return ERR_LOAD_MREMAP_HEAP;
	} else {
		if (heap_size > 0) {
			heap_ptr = sys_mmap((void *) info.heap_addr, heap_size, PROT_NONE, MAP_SHARED | MAP_FIXED, state_fd, heap_offset);
			if (heap_ptr == MAP_FAILED)
				return ERR_LOAD_MMAP_HEAP;

			if (heap_allocated > 0) {
				if (sys_mprotect(heap_ptr, heap_allocated, PROT_READ | PROT_WRITE) != 0)
					return ERR_LOAD_MPROTECT_HEAP;
			}
		} else {
			// Memory address cannot be arbitrary (such as NULL), otherwise
			// it could be followed by other memory mappings.
			heap_ptr = (void *) info.heap_addr;
		}
	}

	void *memory_ptr = heap_ptr + info.globals_size;

	if (sys_close(state_fd) != 0)
		return ERR_LOAD_CLOSE_STATE;

	// Non-blocking I/O.

	if (sys_fcntl(GATE_INPUT_FD, F_SETFL, O_NONBLOCK) != 0)
		return ERR_LOAD_FCNTL_INPUT;

	if (sys_fcntl(GATE_OUTPUT_FD, F_SETFL, O_NONBLOCK) != 0)
		return ERR_LOAD_FCNTL_OUTPUT;

	// Start runtime.

	code *init_routine = GATE_SANDBOX ? &runtime_init : &runtime_init_no_sandbox;

	// _start routine smuggles loader stack address as envp pointer.

	uintptr_t pagemask = info.page_size - 1;
	uintptr_t loader_stack_end = (uintptr_t) envp;
	uintptr_t loader_stack_size = align_size(GATE_LOADER_STACK_SIZE, info.page_size);
	uintptr_t loader_stack = ((loader_stack_end + pagemask) & ~pagemask) - loader_stack_size;

	enter(stack_ptr,
	      stack_limit,
	      runtime_func_addr(runtime_ptr, init_routine),
	      loader_stack,
	      loader_stack_size,
	      runtime_func_addr(runtime_ptr, &signal_handler),
	      runtime_func_addr(runtime_ptr, &signal_restorer),
	      memory_ptr,
	      text_ptr + (uintptr_t) info.init_routine);
}
