static bool strcmp_clock_gettime(const char *name)
{
	if (strlen(name) != 20)
		return false;

	if (((const uint64_t *) name)[0] != 0x635f6f7364765f5fULL) // Little-endian "__vdso_c"
		return false;

	if (((const uint64_t *) name)[1] != 0x7465675f6b636f6cULL) // Little-endian "lock_get"
		return false;

	if (((const uint32_t *) name)[4] != 0x656d6974UL) // Little-endian "time"
		return false;

	return true;
}

static inline void enter(
	void *stack_ptr,
	void *stack_limit,
	uint64_t runtime_init,
	uintptr_t loader_stack,
	size_t loader_stack_size,
	uint64_t signal_handler,
	uint64_t signal_restorer,
	void *memory_ptr,
	void *init_routine)
{
	register void *rax asm("rax") = stack_ptr;
	register void *rbx asm("rbx") = stack_limit;
	register uint64_t rbp asm("rbp") = runtime_init;
	register size_t rsi asm("rsi") = loader_stack_size; // munmap length
	register uintptr_t rdi asm("rdi") = loader_stack;   // munmap addr
	register uint64_t r9 asm("r9") = signal_handler;
	register uint64_t r10 asm("r10") = signal_restorer;
	register void *r14 asm("r14") = memory_ptr;
	register void *r15 asm("r15") = init_routine;

	// clang-format off

	asm volatile(
		// Replace stack.

		"mov  %%rax, %%rsp                          \n"

		// Unmap old stack (ASLR breaks this).

		"mov  $"xstr(SYS_munmap)", %%eax            \n"
		"syscall                                    \n"
		"mov  $"xstr(ERR_LOAD_MUNMAP_STACK)", %%edi \n"
		"test %%rax, %%rax                          \n"
		"jne  sys_exit                              \n"

		// Build sigaction structure on stack.  Using 32 bytes of red
		// zone.

		"mov  %%rsp, %%rsi                          \n"
		"sub  $32, %%rsi                            \n" // sigaction act
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

		// Execute runtime_init.

		"mov  %%rbp, %%rcx                          \n"
		"jmp  retpoline                             \n"
		:
		: "r"(rax), "r"(rbx), "r"(rbp), "r"(rsi), "r"(rdi), "r"(r9), "r"(r10), "r"(r14), "r"(r15));

	// clang-format on

	__builtin_unreachable();
}
