#include <signal.h>
#include <stddef.h>
#include <stdint.h>

#include <fcntl.h>
#include <sys/mman.h>
#include <sys/stat.h>
#include <sys/syscall.h>
#include <sys/types.h>

#include "../defs.h"

#define xstr(s) str(s)
#define str(s)  #s

#define SYS_SA_RESTORER 0x04000000
#define SIGACTION_FLAGS (SA_RESTART | SYS_SA_RESTORER)

// avoiding function prototypes avoids GOT section
extern int runtime_exit;
extern int runtime_start;
extern int signal_handler;
extern int signal_restorer;
extern int trap_handler;

__attribute__ ((noreturn))
static void sys_exit(int status)
{
	asm volatile (
		" syscall \n"
		" int3    \n"
		:
		: "a" (SYS_exit), "D" (status)
	);
	__builtin_unreachable();
}

static ssize_t sys_read(int fd, void *buf, size_t count)
{
	ssize_t retval;

	asm volatile (
		"syscall"
		: "=a" (retval)
		: "a" (SYS_read), "D" (fd), "S" (buf), "d" (count)
		: "cc", "rcx", "r11", "memory"
	);

	return retval;
}

static void *sys_mmap(void *addr, size_t length, int prot, int flags, int fd, off_t offset)
{
	void *retval;

	register void *rdi asm ("rdi") = addr;
	register size_t rsi asm ("rsi") = length;
	register int rdx asm ("rdx") = prot;
	register int r10 asm ("r10") = flags;
	register int r8 asm ("r8") = fd;
	register off_t r9 asm ("r9") = offset;

	asm volatile (
		"syscall"
		: "=a" (retval)
		: "a" (SYS_mmap), "r" (rdi), "r" (rsi), "r" (rdx), "r" (r10), "r" (r8), "r" (r9)
		: "cc", "rcx", "r11", "memory"
	);

	return retval;
}

static int sys_open(const char *pathname, int flags, mode_t mode)
{
	int retval;

	register const char *rdi asm ("rdi") = pathname;
	register int rsi asm ("rsi") = flags;
	register mode_t rdx asm ("rdx") = mode;

	asm volatile (
		"syscall"
		: "=a" (retval)
		: "a" (SYS_open), "r" (rdi), "r" (rsi), "r" (rdx)
		: "cc", "rcx", "r11", "memory"
	);

	return retval;
}

static int sys_close(int fd)
{
	int retval;

	asm volatile (
		"syscall"
		: "=a" (retval)
		: "a" (SYS_close), "D" (fd)
		: "cc", "rcx", "r11", "memory"
	);

	return retval;
}

static int read_full(void *buf, size_t size)
{
	for (size_t pos = 0; pos < size; ) {
		ssize_t len = sys_read(0, buf + pos, size - pos);
		if (len < 0)
			return -1;
		pos += len;
	}

	return 0;
}

__attribute__ ((noreturn))
static void enter(uint64_t page_size, void *text_ptr, void *memory_ptr, void *init_memory_limit, void *grow_memory_limit, void *stack_ptr, void *stack_limit)
{
	register void *rax asm ("rax") = stack_ptr;
	register void *rdx asm ("rdx") = &trap_handler;
	register void *rcx asm ("rcx") = grow_memory_limit;
	register uint64_t rsi asm ("rsi") = GATE_LOADER_STACK_PAGES * page_size;
	register void *r9 asm ("r9") = &signal_handler;
	register void *r10 asm ("r10") = &signal_restorer;
	register uint64_t r11 asm ("r11") = page_size;
	register void *r12 asm ("r12") = text_ptr;
	register void *r13 asm ("r13") = stack_limit;
	register void *r14 asm ("r14") = memory_ptr;
	register void *r15 asm ("r15") = init_memory_limit;

	asm volatile (
		// MMX registers
		"        movq    %%rdx, %%mm0                            \n"
		"        movq    %%rcx, %%mm1                            \n"
		// replace stack
		"        mov     %%rsp, %%rdi                            \n"
		"        mov     %%rax, %%rsp                            \n"
		// unmap old stack (hoping that stack pointer was somewhere in the last frame)
		"        dec     %%r11                                   \n"
		"        add     %%r11, %%rdi                            \n"
		"        not     %%r11                                   \n"
		"        and     %%r11, %%rdi                            \n"
		"        sub     %%rsi, %%rdi                            \n"
		"        mov     $"xstr(SYS_munmap)", %%eax              \n"
		"        syscall                                         \n"
		"        mov     $52, %%edi                              \n"
		"        test    %%rax, %%rax                            \n"
		"        jne     runtime_exit                            \n"
		// register suspend signal handler (using 32 bytes of stack red zone)
		"        mov     $"xstr(SIGUSR1)", %%edi                 \n" // signum
		"        xor     %%edx, %%edx                            \n" // oldact
		"        lea     -32(%%rsp), %%rsi                       \n" // act
		"        mov     %%r9, (%%rsi)                           \n" //   handler
		"        movq    $"xstr(SIGACTION_FLAGS)", 8(%%rsi)      \n" //   flags
		"        mov     %%r10, 16(%%rsi)                        \n" //   restorer
		"        mov     %%rdx, 24(%%rsi)                        \n" //   mask (0)
		"        mov     $8, %%r10                               \n" // mask size
		"        xor     %%r9d, %%r9d                            \n" // clear suspend flag
		"        mov     $"xstr(SYS_rt_sigaction)", %%eax        \n"
		"        syscall                                         \n"
		"        mov     $53, %%edi                              \n"
		"        test    %%rax, %%rax                            \n"
		"        jne     runtime_exit                            \n"
		// execute runtime
		"        jmp     runtime_start                           \n"
		:
		: "r" (rax), "r" (rdx), "r" (rcx), "r" (rsi), "r" (r9), "r" (r10), "r" (r11), "r" (r12), "r" (r13), "r" (r14), "r" (r15)
	);
	__builtin_unreachable();
}

static int main()
{
	struct __attribute__ ((packed)) {
		uint64_t text_addr;
		uint64_t heap_addr;
		uint32_t page_size;
		uint32_t rodata_size;
		uint32_t text_size;
		uint32_t memory_offset;
		uint32_t init_memory_size;
		uint32_t grow_memory_size;
		uint32_t stack_size;
		uint32_t magic_number;
	} info;

	if (read_full(&info, sizeof (info)) != 0)
		return 54;

	if (info.magic_number != GATE_MAGIC_NUMBER)
		return 55;

	if (info.rodata_size > 0) {
		void *ptr = sys_mmap((void *) GATE_RODATA_ADDR, info.rodata_size, PROT_READ, MAP_PRIVATE|MAP_FIXED|MAP_NORESERVE, GATE_MAPS_FD, 0);
		if (ptr != (void *) GATE_RODATA_ADDR)
			return 56;
	}

	void *text_ptr = sys_mmap((void *) info.text_addr, info.text_size, PROT_EXEC, MAP_PRIVATE|MAP_NORESERVE|MAP_FIXED, GATE_MAPS_FD, info.rodata_size);
	if (text_ptr != (void *) info.text_addr)
		return 57;

	size_t globals_memory_offset = (size_t) info.rodata_size + (size_t) info.text_size;
	size_t globals_memory_size = info.memory_offset + info.grow_memory_size;

	void *memory_ptr = NULL;

	if (globals_memory_size > 0) {
		void *ptr = sys_mmap((void *) info.heap_addr, globals_memory_size, PROT_READ|PROT_WRITE, MAP_SHARED|MAP_FIXED|MAP_NORESERVE, GATE_MAPS_FD, globals_memory_offset);
		if (ptr != (void *) info.heap_addr)
			return 58;

		memory_ptr = ptr + info.memory_offset;
	}

	void *init_memory_limit = memory_ptr + info.init_memory_size;
	void *grow_memory_limit = memory_ptr + info.grow_memory_size;

	size_t stack_offset = globals_memory_offset + globals_memory_size;

	void *stack_buf = sys_mmap(NULL, info.stack_size, PROT_READ|PROT_WRITE, MAP_SHARED|MAP_NORESERVE|MAP_STACK, GATE_MAPS_FD, stack_offset);
	if (stack_buf == MAP_FAILED)
		return 59;

	void *stack_limit = stack_buf + GATE_SIGNAL_STACK_RESERVE;
	void *stack_ptr = stack_buf + info.stack_size;

	if (sys_close(GATE_MAPS_FD) != 0)
		return 60;

	int nonblock_fd = sys_open(GATE_BLOCK_PATH, O_RDONLY|O_CLOEXEC|O_NONBLOCK, 0);
	if (nonblock_fd != GATE_NONBLOCK_FD)
		return 61;

	enter(info.page_size, text_ptr, memory_ptr, init_memory_limit, grow_memory_limit, stack_ptr, stack_limit);
}

__attribute__ ((noreturn))
void _start()
{
	sys_exit(main());
}
