// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <csignal>
#include <cstdint>

#include <sys/syscall.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <unistd.h>

#include "attribute.hpp"
#include "syscall.hpp"

using runtime::syscall;

namespace {

struct Sigaction {
	void (*handler)(int);
	int flags;
	void (*restorer)(void);
	sigset_t mask;
};

const int sa_restorer = 0x04000000;

NORETURN void sys_exit(int status)
{
	syscall1(SYS_exit, status);
	__builtin_unreachable();
}

int sys_fork()
{
	return syscall6(SYS_clone, SIGCHLD, 0, 0, 0, 0, 0);
}

int sys_sigaction(int signum, const Sigaction* act, Sigaction* oldact)
{
	size_t masksize = 8;
	return syscall4(SYS_rt_sigaction, signum, (uintptr_t) act, (uintptr_t) oldact, masksize);
}

pid_t sys_wait4(pid_t pid, int* wstatus, int options, rusage* rusage)
{
	return syscall4(SYS_wait4, pid, (uintptr_t) wstatus, options, (uintptr_t) rusage);
}

ssize_t sys_write(int fd, const void* buf, size_t count)
{
	return syscall3(SYS_write, fd, (uintptr_t) buf, count);
}

void output(uint64_t i)
{
	if (sys_write(STDOUT_FILENO, &i, sizeof i) != sizeof i)
		sys_exit(2);
}

volatile uint64_t scan_addr;
volatile uint64_t handbase_addr;

void segfault_handler(int signum UNUSED)
{
	output(scan_addr);

	uint64_t hand_addr;
#ifdef __amd64__
	asm volatile(
		"mov %%rsp, %%rax"
		: "=a"(hand_addr)::);
#else
	register uint64_t hand_addr_r asm("x0");
	asm volatile(
		"mov x0, sp"
		: "=r"(hand_addr_r)::);
	hand_addr = hand_addr_r;
#endif
	output(handbase_addr - hand_addr);

	sys_exit(0);
}

NORETURN void scan(uint64_t addr, uint64_t step)
{
	while (1) {
#ifdef __amd64__
		asm volatile(
			"mov %%rsp, %%rax"
			: "=a"(handbase_addr)::);
#else
		register uint64_t handbase_addr_r asm("x0");
		asm volatile(
			"mov x0, sp"
			: "=r"(handbase_addr_r)::);
		handbase_addr = handbase_addr_r;
#endif

		scan_addr = addr;
		*reinterpret_cast<uint64_t volatile*>(addr); // read memory
		addr += step;
	}
}

int inspect(uintptr_t init_addr)
{
	output(init_addr);

	const Sigaction sa = {
		.handler = segfault_handler,
		.flags = sa_restorer, // it is never needed
		.restorer = nullptr,
		.mask = {0},
	};

	if (sys_sigaction(SIGSEGV, &sa, nullptr) != 0)
		sys_exit(3);

	if (sys_sigaction(SIGBUS, &sa, nullptr) != 0)
		sys_exit(4);

	pid_t pid = sys_fork();
	if (pid < 0)
		sys_exit(5);

	if (pid == 0) {
		scan(init_addr, -sizeof(uint64_t));
		sys_exit(5);
	} else {
		int status;
		if (sys_wait4(-1, &status, 0, nullptr) != pid)
			sys_exit(6);
		if (status != 0)
			sys_exit(7);

		scan(init_addr, sizeof(uint64_t));
	}
}

} // namespace

int main(int argc UNUSED, char** argv UNUSED, char** envp)
{
	// _start smuggles stack address as envp.
	return inspect(uintptr_t(envp));
}

extern "C" void* memset(void* s, int c, size_t n)
{
	for (size_t i = 0; i < n; i++)
		reinterpret_cast<uint8_t*>(s)[i] = c;
	return s;
}
