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

static bool strcmp_clock_gettime(const char *name)
{
	if (strlen(name) != 22)
		return false;

	if (((const uint64_t *) name)[0] != 0x6c656e72656b5f5fULL) // Little-endian "__kernel"
		return false;

	if (((const uint64_t *) name)[1] != 0x675f6b636f6c635fULL) // Little-endian "_clock_g"
		return false;

	if (((const uint32_t *) name)[4] != 0x69747465UL) // Little-endian "etti"
		return false;

	if (((const uint16_t *) name)[10] != 0x656d) // Little-endian "me"
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
	register uintptr_t r0 asm("r0") = loader_stack;   // munmap addr
	register size_t r1 asm("r1") = loader_stack_size; // munmap length
	register uint64_t r4 asm("r4") = signal_handler;
	register uint64_t r5 asm("r5") = SIGACTION_FLAGS;
	register uint64_t r6 asm("r6") = signal_restorer;
	register uint64_t r7 asm("r7") = runtime_init;
	register void *r26 asm("r26") = memory_ptr;
	register void *r27 asm("r27") = init_routine;
	register void *r28 asm("r28") = stack_limit;
	register void *r29 asm("r29") = stack_ptr;

	// clang-format off

	asm volatile(
		// Replace stack.

		"sub  sp, x28, #128+16                   \n" // Real stack pointer before red zone.
		"lsr  x28, x28, #4                       \n" // Stack limit >> 4.

		// Unmap old stack (ASLR breaks this).

		"mov  w8, "xstr(SYS_munmap)"             \n"
		"svc  #0                                 \n"
		"cmp  w0, #0                             \n"
		"mov  w0, #"xstr(ERR_LOAD_MUNMAP_STACK)" \n"
		"b.ne sys_exit                           \n"

		// Build sigaction structure on stack.  Using 32 bytes of red
		// zone.

		"mov  x1, sp                             \n" // sigaction act
		"str  x4, [x1, #0]                       \n" // sa_handler
		"str  x5, [x1, #8]                       \n" // sa_flags
		"str  x6, [x1, #16]                      \n" // sa_restorer
		"str  xzr, [x1, #24]                     \n" // sa_mask

		"mov  x2, #0                             \n" // sigaction oldact
		"mov  x3, #8                             \n" // sigaction mask size

		// Async I/O signal handler.

		"mov  w0, #"xstr(SIGIO)"                 \n" // sigaction signum
		"mov  w8, #"xstr(SYS_rt_sigaction)"      \n"
		"svc  #0                                 \n"
		"cmp  w0, #0                             \n"
		"mov  w0, #"xstr(ERR_LOAD_SIGACTION)"    \n"
		"b.ne sys_exit                           \n"

		// Segmentation fault signal handler.

		"mov  w0, #"xstr(SIGSEGV)"               \n" // sigaction signum
		"mov  w8, #"xstr(SYS_rt_sigaction)"      \n"
		"svc  #0                                 \n"
		"cmp  w0, #0                             \n"
		"mov  w0, #"xstr(ERR_LOAD_SIGACTION)"    \n"
		"b.ne sys_exit                           \n"

		// Suspend signal handler.

		"mov  w0, #"xstr(SIGXCPU)"               \n" // sigaction signum
		"mov  w8, #"xstr(SYS_rt_sigaction)"      \n"
		"svc  #0                                 \n"
		"cmp  w0, #0                             \n"
		"mov  w0, #"xstr(ERR_LOAD_SIGACTION)"    \n"
		"b.ne sys_exit                           \n"

		// Execute runtime_init.

		"br   x7                                 \n"
		:
		: "r"(r0), "r"(r1), "r"(r4), "r"(r5), "r"(r6), "r"(r7), "r"(r26), "r"(r27), "r"(r28), "r"(r29));

	// clang-format on

	__builtin_unreachable();
}
