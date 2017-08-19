// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <signal.h>
#include <stdint.h>

#include <sys/syscall.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <unistd.h>

static int sys_fork()
{
	int retval;

	asm (
		"syscall"
		: "=a" (retval)
		: "a" (SYS_fork)
	);

	return retval;
}

static void output(uint64_t i)
{
	if (write(STDOUT_FILENO, &i, sizeof (i)) != sizeof (i))
		_exit(2);
}

static volatile uint64_t scan_addr;

static void segfault_handler(int signum)
{
	output(scan_addr);
	_exit(0);
}

__attribute__ ((noreturn))
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

	asm (
		"movq %%mm7, %%rax"
		: "=a" (init_addr)
	);

	output(init_addr);

	if (signal(SIGSEGV, segfault_handler) == SIG_ERR)
		_exit(3);

	if (signal(SIGBUS, segfault_handler) == SIG_ERR)
		_exit(4);

	pid_t pid = sys_fork();
	if (pid == 0)
		scan(init_addr, -sizeof (uint64_t));
	if (pid <= 0)
		_exit(5);

	int status;
	if (wait(&status) != pid)
		_exit(6);
	if (status != 0)
		_exit(7);

	scan(init_addr, sizeof (uint64_t));
}
