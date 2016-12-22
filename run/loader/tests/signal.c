#include <signal.h>
#include <stdio.h>
#include <unistd.h>

#include "../../defs.h"

void signal_handler(int signum);

static void *stackptr_signal;

static void test_handler(int signum)
{
	asm volatile (
		"       mov      %%rsp, %%rax    \n"
		: "=a" (stackptr_signal)
		:
		:
	);
}

int main(void)
{
	// TODO: static assert
	if (GATE_SIGNAL_STACK_R9_OFFSET != sizeof (void *) + (size_t) &(((struct ucontext *) 0)->uc_mcontext.gregs[1]))
		return -1;

	void *stackptr_main;

	asm volatile (
		"       mov      %%rsp, %%rax    \n"
		: "=a" (stackptr_main)
		:
		:
	);

	printf("stack pointer in main   = %p\n", stackptr_main);

	signal(SIGALRM, test_handler);
	alarm(1);
	pause();

	printf("stack pointer in signal = %p\n", stackptr_signal);
	printf("difference = %ld\n", (long) stackptr_main - (long) stackptr_signal);

	signal(SIGALRM, signal_handler);
	alarm(1);

	unsigned long rounds;

	asm volatile (
		"        xor     %%rax, %%rax    \n"
		"        xor     %%r9, %%r9      \n"
		".Lloop:                         \n"
		"        inc     %%rax           \n"
		"        test    %%r9, %%r9      \n"
		"        je      .Lloop          \n"
		: "=a" (rounds)
		:
		: "r9"
	);

	printf("rounds = %ld\n", rounds);
	return 0;
}
