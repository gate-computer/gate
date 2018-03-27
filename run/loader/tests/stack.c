// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <signal.h>
#include <stdint.h>

#include <sys/syscall.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <unistd.h>

#define NORETURN __attribute__((noreturn))

#define SYS_SA_RESTORER 0x04000000

struct sys_sigaction {
	void (*handler)(int);
	int flags;
	void (*restorer)(void);
	sigset_t mask;
};

NORETURN
static void sys_exit(int status)
{
	asm(
		"syscall"
		:
		: "a"(SYS_exit), "D"(status));
	__builtin_unreachable();
}

static int sys_fork()
{
	int retval;

	asm(
		"syscall"
		: "=a"(retval)
		: "a"(SYS_fork));

	return retval;
}

static int sys_sigaction(
	int signum,
	const struct sys_sigaction *act,
	struct sys_sigaction *oldact)
{
	int retval;

	register int rdi asm("rdi") = signum;
	register const struct sys_sigaction *rsi asm("rsi") = act;
	register struct sys_sigaction *rdx asm("rdx") = oldact;
	register long r10 asm("r10") = 8; // mask size

	asm volatile(
		"syscall"
		: "=a"(retval)
		: "a"(SYS_rt_sigaction), "r"(rdi), "r"(rsi), "r"(rdx), "r"(r10)
		: "cc", "rcx", "r11", "memory");

	return retval;
}

static pid_t sys_wait4(
	pid_t pid, int *wstatus, int options, struct rusage *rusage)
{
	pid_t retval;

	register pid_t rdi asm("rdi") = pid;
	register int *rsi asm("rsi") = wstatus;
	register int rdx asm("rdx") = options;
	register struct rusage *r10 asm("r10") = rusage;

	asm volatile(
		"syscall"
		: "=a"(retval)
		: "a"(SYS_wait4), "r"(rdi), "r"(rsi), "r"(rdx), "r"(r10)
		: "cc", "rcx", "r11", "memory");

	return retval;
}

static ssize_t sys_write(int fd, const void *buf, size_t count)
{
	ssize_t retval;

	asm volatile(
		"syscall"
		: "=a"(retval)
		: "a"(SYS_write), "D"(fd), "S"(buf), "d"(count)
		: "cc", "rcx", "r11", "memory");

	return retval;
}

static void output(uint64_t i)
{
	if (sys_write(STDOUT_FILENO, &i, sizeof i) != sizeof i)
		sys_exit(2);
}

static volatile uint64_t scan_addr;

static void segfault_handler(int signum)
{
	output(scan_addr);
	sys_exit(0);
}

NORETURN
static void scan(uint64_t addr, uint64_t step)
{
	while (1) {
		scan_addr = addr;
		*(volatile uint64_t *) addr; // read memory
		addr += step;
	}
}

int main(int argc, char **argv, char **envp)
{
	uint64_t init_addr;

	asm(
		"movq %%mm7, %%rax"
		: "=a"(init_addr));

	output(init_addr);

	const struct sys_sigaction sa = {
		.handler = segfault_handler,
		.flags = SYS_SA_RESTORER, // it is never needed
	};

	if (sys_sigaction(SIGSEGV, &sa, NULL) != 0)
		sys_exit(3);

	if (sys_sigaction(SIGBUS, &sa, NULL) != 0)
		sys_exit(4);

	pid_t pid = sys_fork();
	if (pid < 0)
		sys_exit(5);

	if (pid == 0) {
		scan(init_addr, -sizeof(uint64_t));
		sys_exit(5);
	} else {
		int status;
		if (sys_wait4(-1, &status, 0, NULL) != pid)
			sys_exit(6);
		if (status != 0)
			sys_exit(7);

		scan(init_addr, sizeof(uint64_t));
	}
}
