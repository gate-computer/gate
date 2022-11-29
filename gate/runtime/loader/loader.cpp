// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <csignal>
#include <cstdbool>
#include <cstddef>
#include <cstdint>
#include <cstring>
#include <ctime>

#include <elf.h>
#include <fcntl.h>
#include <sys/mman.h>
#include <sys/prctl.h>
#include <sys/resource.h>
#include <sys/socket.h>
#include <sys/syscall.h>
#include <sys/time.h>
#include <sys/types.h>

#include "align.hpp"
#include "attribute.hpp"
#include "errors.gen.hpp"
#include "runtime.hpp"
#include "syscall.hpp"

#include "debug.hpp"

#define SYS_SA_RESTORER 0x04000000
#define SIGACTION_FLAGS (SYS_SA_RESTORER | SA_SIGINFO)

#ifdef __ANDROID__
#define ANDROID 1
#define MAYBE_MAP_FIXED 0
#else
#define ANDROID 0
#define MAYBE_MAP_FIXED MAP_FIXED
#endif

#include "loader.hpp"

extern "C" {

// Avoiding function prototypes avoids GOT section.
typedef const class {
	uint8_t dummy;
} code;

extern code current_memory;
extern code grow_memory;
extern code retpoline;
extern code rt_debug;
extern code rt_flags;
extern code rt_nop;
extern code rt_poll;
extern code rt_random;
extern code rt_read8;
extern code rt_read;
extern code rt_start;
extern code rt_start_no_sandbox;
extern code rt_text_end;
extern code rt_text_start;
extern code rt_time;
extern code rt_timemask;
extern code rt_trap;
extern code rt_write8;
extern code rt_write;
extern code signal_handler;
extern code signal_restorer;
extern code sys_exit;
extern code trap_handler;

} // extern "C"

namespace runtime::loader {

uintptr_t rt_func_addr(void const* new_base, code* func_ptr)
{
	return uintptr_t(new_base) + uintptr_t(func_ptr) - uintptr_t(&rt_text_start);
}

int sys_close(int fd)
{
	return syscall(SYS_close, fd);
}

int sys_fcntl(int fd, int cmd, int arg)
{
	return syscall(SYS_fcntl, fd, cmd, arg);
}

void* sys_mmap(void* addr, size_t length, int prot, int flags, int fd, off_t offset)
{
	intptr_t ret = syscall(SYS_mmap, uintptr_t(addr), length, prot, flags, fd, offset);
	if (ret < 0 && ret > -4096)
		return MAP_FAILED;
	return reinterpret_cast<void*>(ret);
}

int sys_mprotect(void* addr, size_t len, int prot)
{
	return syscall(SYS_mprotect, uintptr_t(addr), len, prot);
}

void* sys_mremap(void* old_addr, size_t old_size, size_t new_size, int flags)
{
	intptr_t ret = syscall(SYS_mremap, uintptr_t(old_addr), old_size, new_size, flags);
	if (ret < 0 && ret > -4096)
		return MAP_FAILED;
	return reinterpret_cast<void*>(ret);
}

int sys_personality(unsigned long persona)
{
	return syscall(SYS_personality, persona);
}

int sys_prctl(int option, unsigned long arg2)
{
	return syscall(SYS_prctl, option, arg2);
}

ssize_t sys_read(int fd, void* buf, size_t count)
{
	return syscall(SYS_read, fd, uintptr_t(buf), count);
}

ssize_t sys_recvmsg(int sockfd, msghdr* msg, int flags)
{
	return syscall(SYS_recvmsg, sockfd, uintptr_t(msg), flags);
}

int sys_setrlimit(int resource, rlim_t limit)
{
	const rlimit buf = {limit, limit};
	return syscall(SYS_setrlimit, resource, uintptr_t(&buf));
}

// This is like imageInfo in runtime/process.go
struct ImageInfo {
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
	uint32_t start_addr;
	uint32_t entry_addr;
	uint32_t time_mask;
	uint64_t monotonic_time;
	uint64_t magic_number_2;
} PACKED;

// This is like stackVars in image/instance.go
struct StackVars {
	uint32_t stack_unused;
	uint32_t current_memory_pages; // WebAssembly pages.
	uint64_t monotonic_time_snapshot;
	int32_t random_avail;
	uint32_t bits; // 0x1 = suspended | 0x2 = don't modify suspend reg | 0x4 = gate_io flag: started or resumed
	uint64_t text_addr;
	uint64_t result[2]; // [0] is int, [1] is float.
	uint64_t magic[2];
} PACKED;

int receive_info(ImageInfo* buf, int* text_fd, int* state_fd)
{
	iovec iov = {buf, sizeof *buf};

	union {
		char buf[CMSG_SPACE(3 * sizeof(int))];
		cmsghdr alignment;
	} ctl;

	msghdr msg;
	memset(&msg, 0, sizeof msg);

	msg.msg_iov = &iov;
	msg.msg_iovlen = 1;
	msg.msg_control = ctl.buf;
	msg.msg_controllen = sizeof ctl.buf;

	auto n = sys_recvmsg(GATE_INPUT_FD, &msg, MSG_CMSG_CLOEXEC);
	if (n < 0)
		return -1;

	if (n != sizeof(ImageInfo))
		return -1;

	if (msg.msg_flags & MSG_CTRUNC)
		return -1;

	auto cmsg = CMSG_FIRSTHDR(&msg);
	if (cmsg == nullptr)
		return -1;

	if (cmsg->cmsg_level != SOL_SOCKET)
		return -1;

	if (cmsg->cmsg_type != SCM_RIGHTS)
		return -1;

	auto fds = reinterpret_cast<int const*>(CMSG_DATA(cmsg));
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

auto elf_section(Elf64_Ehdr const* elf, unsigned index)
{
	return reinterpret_cast<Elf64_Shdr const*>(uintptr_t(elf) + elf->e_shoff + elf->e_shentsize * index);
}

auto elf_string(Elf64_Ehdr const* elf, unsigned strtab_index, unsigned str_index)
{
	auto strtab = elf_section(elf, strtab_index);
	return reinterpret_cast<char const*>(uintptr_t(elf) + strtab->sh_offset + str_index);
}

int main(Elf64_Ehdr const* vdso, uintptr_t loader_stack_end)
{
	if (sys_prctl(PR_SET_PDEATHSIG, SIGKILL) != 0)
		return ERR_LOAD_PDEATHSIG;

	uintptr_t clock_gettime_addr = 0;

	for (unsigned i = 0; i < vdso->e_shnum; i++) {
		auto shdr = elf_section(vdso, i);
		if (shdr->sh_type != SHT_DYNSYM)
			continue;

		for (uint64_t off = 0; off < shdr->sh_size; off += shdr->sh_entsize) {
			auto vdso_addr = uintptr_t(vdso);
			auto sym = reinterpret_cast<Elf64_Sym const*>(vdso_addr + shdr->sh_offset + off);

			auto name = elf_string(vdso, shdr->sh_link, sym->st_name);
			if (!strcmp_clock_gettime(name))
				continue;

			clock_gettime_addr = vdso_addr + sym->st_value;
			goto clock_gettime_found;
		}
	}
	return ERR_LOAD_NO_CLOCK_GETTIME;

clock_gettime_found:
	// Miscellaneous preparations.

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

	if (GATE_SANDBOX) {
		if (sys_prctl(PR_SET_DUMPABLE, 0) != 0)
			return ERR_LOAD_PRCTL_NOT_DUMPABLE;
	}

	// Image info and file descriptors

	ImageInfo info;
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

	timespec t;

	auto gettime = reinterpret_cast<int (*)(clockid_t, timespec*)>(clock_gettime_addr);
	if (gettime(CLOCK_MONOTONIC_COARSE, &t) != 0)
		return ERR_LOAD_CLOCK_GETTIME;

	t.tv_sec--; // Ensure that rt_time never returns zero timestamp.
	t.tv_nsec &= uint64_t(info.time_mask);
	auto local_monotonic_time_base = uint64_t(t.tv_sec) * 1000000000ULL + uint64_t(t.tv_nsec);

	// RT: text at start, import vector at end (and maybe space for text)

	auto rt_map_size = info.page_size + (ANDROID ? info.text_size : 0);

	auto rt = sys_mmap(reinterpret_cast<void*>(info.text_addr - uintptr_t(info.page_size)), rt_map_size, PROT_READ | PROT_WRITE, MAP_PRIVATE | MAP_ANONYMOUS | MAYBE_MAP_FIXED, -1, 0);
	if (rt == MAP_FAILED)
		return ERR_LOAD_MMAP_VECTOR;

	auto rt_size = uintptr_t(&rt_text_end) - uintptr_t(&rt_text_start);
	memcpy(rt, &rt_text_start, rt_size);

	auto vector_end = reinterpret_cast<uint64_t*>(uintptr_t(rt) + info.page_size);

	// Text

	void* text_ptr = vector_end;

	if (ANDROID) {
		if (sys_read(text_fd, text_ptr, info.text_size) != info.text_size)
			return ERR_LOAD_READ_TEXT;
	} else {
		auto ret = sys_mmap(text_ptr, info.text_size, PROT_READ | PROT_EXEC, MAP_PRIVATE | MAP_FIXED, text_fd, 0);
		if (ret == MAP_FAILED)
			return ERR_LOAD_MMAP_TEXT;
	}

	if (sys_close(text_fd) != 0)
		return ERR_LOAD_CLOSE_TEXT;

	// Stack

	auto stack_buf = sys_mmap(reinterpret_cast<void*>(info.stack_addr), info.stack_size, PROT_READ | PROT_WRITE, MAP_SHARED | MAYBE_MAP_FIXED, state_fd, 0);
	if (stack_buf == MAP_FAILED)
		return ERR_LOAD_MMAP_STACK;

	auto vars = reinterpret_cast<StackVars volatile*>(stack_buf);
	vars->stack_unused = 0; // Invalidate state (in case of re-entry).
	vars->current_memory_pages = info.init_memory_size >> 16;
	vars->monotonic_time_snapshot = info.monotonic_time;
	vars->random_avail = sizeof info.random;
	vars->bits = 0x4; // Started or resumed.
	vars->text_addr = uintptr_t(text_ptr);
	for (unsigned i = 0; i < sizeof vars->result / sizeof vars->result[0]; i++)
		vars->result[i] = 0x5adfad0cafe;
	for (unsigned i = 0; i < sizeof vars->magic / sizeof vars->magic[0]; i++)
		vars->magic[i] = GATE_STACK_MAGIC;

	auto stack_limit = uintptr_t(stack_buf) + GATE_STACK_LIMIT_OFFSET;
	auto stack_ptr = reinterpret_cast<uint64_t*>(stack_buf) + info.stack_unused / 8;

	if (info.stack_unused == info.stack_size) {
		// Synthesize initial stack frame for start or entry routine
		// (checked in runtime/process.go).
		*--stack_ptr = info.entry_addr;
		*--stack_ptr = info.start_addr;
	}

	// Globals and memory

	auto heap_offset = size_t(info.stack_size);
	auto heap_allocated = size_t(info.globals_size) + size_t(info.init_memory_size);
	auto heap_size = size_t(info.globals_size) + size_t(info.grow_memory_size);
	void* heap_ptr;

	if (ANDROID) {
		auto space = size_t(info.globals_size) + MEMORY_ADDRESS_RANGE;

		heap_ptr = sys_mmap(reinterpret_cast<void*>(info.heap_addr), space, PROT_READ | PROT_WRITE, MAP_SHARED, state_fd, heap_offset);
		if (heap_ptr == MAP_FAILED)
			return ERR_LOAD_MMAP_HEAP;

		auto ret = sys_mremap(heap_ptr, space, heap_allocated, 0);
		if (ret == MAP_FAILED || ret != heap_ptr)
			return ERR_LOAD_MREMAP_HEAP;
	} else {
		if (heap_size > 0) {
			heap_ptr = sys_mmap(reinterpret_cast<void*>(info.heap_addr), heap_size, PROT_NONE, MAP_SHARED | MAP_FIXED, state_fd, heap_offset);
			if (heap_ptr == MAP_FAILED)
				return ERR_LOAD_MMAP_HEAP;

			if (heap_allocated > 0) {
				if (sys_mprotect(heap_ptr, heap_allocated, PROT_READ | PROT_WRITE) != 0)
					return ERR_LOAD_MPROTECT_HEAP;
			}
		} else {
			// Memory address cannot be arbitrary (such as null), otherwise it
			// could be followed by other memory mappings.
			heap_ptr = reinterpret_cast<void*>(info.heap_addr);
		}
	}

	auto memory_addr = uintptr_t(heap_ptr) + info.globals_size;

	if (sys_close(state_fd) != 0)
		return ERR_LOAD_CLOSE_STATE;

	// Vector; runtime/text protection

	auto debug_func = debug_flag ? &rt_debug : &rt_nop;

	// These assignments reflect the functions map in runtime/abi/rt/rt.go
	// and rtFunctions map in runtime/abi/abi.go
	// TODO: check that runtime and vector contents don't overlap
	*(vector_end - 21) = rt_func_addr(rt, &rt_flags);
	*(vector_end - 20) = rt_func_addr(rt, &rt_timemask);
	*(vector_end - 19) = rt_func_addr(rt, &rt_write8);
	*(vector_end - 18) = rt_func_addr(rt, &rt_read8);
	*(vector_end - 17) = rt_func_addr(rt, &rt_trap);
	*(vector_end - 16) = rt_func_addr(rt, debug_func);
	*(vector_end - 15) = rt_func_addr(rt, &rt_write);
	*(vector_end - 14) = rt_func_addr(rt, &rt_read);
	*(vector_end - 13) = rt_func_addr(rt, &rt_poll);
	*(vector_end - 12) = rt_func_addr(rt, &rt_time);
	*(vector_end - 11) = clock_gettime_addr;
	*(vector_end - 10) = local_monotonic_time_base;
	*(vector_end - 9) = info.time_mask;
	*(vector_end - 8) = info.random[0];
	*(vector_end - 7) = info.random[1];
	*(vector_end - 6) = rt_func_addr(rt, &rt_random);
	*(vector_end - 5) = info.grow_memory_size >> 16;
	*(vector_end - 4) = memory_addr;
	*(vector_end - 3) = rt_func_addr(rt, &current_memory);
	*(vector_end - 2) = rt_func_addr(rt, &grow_memory);
	*(vector_end - 1) = rt_func_addr(rt, &trap_handler);

	if (sys_mprotect(rt, rt_map_size, PROT_READ | PROT_EXEC) != 0)
		return ERR_LOAD_MPROTECT_VECTOR;

	// Non-blocking I/O.

	if (sys_fcntl(GATE_INPUT_FD, F_SETFL, O_NONBLOCK) != 0)
		return ERR_LOAD_FCNTL_INPUT;

	if (sys_fcntl(GATE_OUTPUT_FD, F_SETFL, O_NONBLOCK) != 0)
		return ERR_LOAD_FCNTL_OUTPUT;

	// Start runtime.

	auto pagemask = uintptr_t(info.page_size) - 1;
	auto loader_stack_size = align_size(GATE_LOADER_STACK_SIZE, info.page_size);
	auto loader_stack = ((loader_stack_end + pagemask) & ~pagemask) - loader_stack_size;

	stack_ptr -= 9;
	stack_ptr[0] = stack_limit;
	stack_ptr[1] = loader_stack;
	stack_ptr[2] = loader_stack_size;
	stack_ptr[3] = rt_func_addr(rt, &signal_handler);
	stack_ptr[4] = SIGACTION_FLAGS;
	stack_ptr[5] = rt_func_addr(rt, &signal_restorer);
	stack_ptr[6] = 0; // Signal mask.
	stack_ptr[7] = uintptr_t(text_ptr) + info.init_routine;
	stack_ptr[8] = rt_func_addr(rt, GATE_SANDBOX ? &rt_start : &rt_start_no_sandbox);

	enter_rt(stack_ptr, stack_limit);
}

template <typename T>
void copy(T* dest, T const* src, size_t n)
{
	for (size_t i = 0; i < n; i++)
		*dest++ = *src++;
}

} // namespace runtime::loader

extern "C" {

void* memcpy(void* dest, void const* src, size_t n)
{
	// This is used to copy rt text, so put medium effort into optimization.

	using runtime::loader::copy;

	struct Block64 {
		uint64_t data[8];
	} PACKED;

	auto d = reinterpret_cast<Block64*>(dest);
	auto s = reinterpret_cast<Block64 const*>(src);
	size_t blocks = n / 64;
	copy(d, s, blocks);

	size_t tail = n & 63;
	if (tail) {
		d += blocks;
		s += blocks;
		copy(reinterpret_cast<uint8_t*>(d), reinterpret_cast<uint8_t const*>(s), tail);
	}

	return dest;
}

void* memset(void* dest, int c, size_t n)
{
	auto d = reinterpret_cast<uint8_t*>(dest);
	for (size_t i = 0; i < n; i++)
		d[i] = c;
	return dest;
}

size_t strlen(char const* s)
{
	size_t n = 0;
	while (*s++)
		n++;
	return n;
}

} // extern "C"

SECTION(".text")
int main(int argc UNUSED, char** argv, char** envp)
{
	// _start smuggles vDSO ELF address as argv and stack address as envp.
	return runtime::loader::main(reinterpret_cast<Elf64_Ehdr const*>(argv), uintptr_t(envp));
}
