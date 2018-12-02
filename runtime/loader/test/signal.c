// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#define _GNU_SOURCE

#include <assert.h>
#include <signal.h>
#include <stdio.h>

#include <ucontext.h>
#include <unistd.h>

#include "runtime.h"

void signal_handler(int signum);

static void *stackptr_signal;

static void test_handler(int signum)
{
	asm volatile(
		"       mov      %%rsp, %%rax    \n"
		: "=a"(stackptr_signal)
		:
		:);
}

int main()
{
	static_assert(GATE_SIGNAL_STACK_SUSPEND_REG_OFFSET == sizeof(void *) + (size_t) & (((ucontext_t *) 0)->uc_mcontext.gregs[REG_RBX]), "position of saved suspend register on signal stack");

	void *stackptr_main;

	asm volatile(
		"       mov      %%rsp, %%rax    \n"
		: "=a"(stackptr_main)
		:
		:);

	printf("stack pointer in main   = %p\n", stackptr_main);

	signal(SIGALRM, test_handler);
	alarm(1);
	pause();

	printf("stack pointer in signal = %p\n", stackptr_signal);
	printf("difference = %ld\n", (long) stackptr_main - (long) stackptr_signal);

	signal(SIGALRM, signal_handler);
	alarm(1);

	unsigned long rounds;

	asm volatile(
		"        xor     %%rax, %%rax    \n"
		"        xor     %%rbx, %%rbx    \n"
		".Lloop:                         \n"
		"        inc     %%rax           \n"
		"        test    %%rbx, %%rbx    \n"
		"        je      .Lloop          \n"
		: "=a"(rounds)
		:
		: "rbx");

	printf("rounds = %ld\n", rounds);
	return 0;
}
