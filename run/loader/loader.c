#include <stddef.h>
#include <stdint.h>

#include <sys/mman.h>
#include <sys/prctl.h>
#include <sys/syscall.h>
#include <sys/types.h>
#include <linux/seccomp.h>
#include <asm/prctl.h>

#include "../../include/gate/args.h"
#include "../stack.h"
#include "abi.h"

#define GATE_BSS_START 0x100001000
#define GATE_BSS_SIZE  4096

#define GATE_GOT_START 0x100201000 // XXX
#define GATE_GOT_SIZE  4096

#define xstr(s) str(s)
#define str(s)  #s

__attribute__ ((section (".abi"))) void abi_exit(int);
__attribute__ ((section (".abi"))) size_t abi_recv(long, void *, size_t);
__attribute__ ((section (".abi"))) size_t abi_send(long, const void *, size_t);

static uint64_t indirect_func_count;
static uint32_t *indirect_func_array;

__attribute__ ((noreturn))
static void sys_exit(int status)
{
	asm volatile (
		" syscall \n"
		" hlt     \n"
		:
		: "a" (SYS_exit), "D" (status)
	);
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

static int sys_munmap(void *addr, size_t length)
{
	int retval;

	asm volatile (
		"syscall"
		: "=a" (retval)
		: "a" (SYS_munmap), "D" (addr), "S" (length)
		: "cc", "rcx", "r11", "memory"
	);

	return retval;
}

static int sys_mprotect(void *addr, size_t len, int prot)
{
	int retval;

	asm volatile (
		"syscall"
		: "=a" (retval)
		: "a" (SYS_mprotect), "D" (addr), "S" (len), "d" (prot)
		: "cc", "rcx", "r11", "memory"
	);

	return retval;
}

static int sys_arch_prctl(int code, unsigned long addr)
{
	int retval;

	asm volatile (
		"syscall"
		: "=a" (retval)
		: "a" (SYS_arch_prctl), "D" (code), "S" (addr)
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

static void *indirect_call_check(void *func_ptr)
{
	uint32_t func_addr = (uintptr_t) func_ptr;
	register uint32_t *array = indirect_func_array;

	// there are always some abi functions in the array, so it's not empty
	uint64_t min = 0;
	uint64_t max = indirect_func_count - 1;

	while (min <= max) {
		uint64_t middle = (min + max) >> 1; // indices are not larger than 32 bits

		uint32_t addr = array[middle];
		if (addr == func_addr)
			goto ok;

		if (addr < func_addr)
			min = middle + 1;
		else
			max = middle - 1;
	}

	sys_exit(10);

ok:
	// with luck, wipe array pointer register... TODO: reimplement whole function in asm
	asm volatile (
		"xor %0, %0"
		:
		: "r" (array)
	);

	return (void *) (uintptr_t) func_addr;
}

__attribute__ ((noreturn))
static void enter(uint64_t page_size, void *safe_stack_ptr, void (*func)(uint32_t *))
{
	register void *r10 asm ("r10") = safe_stack_ptr;
	register uint64_t r11 asm ("r11") = page_size - 1;
	register uint64_t rsi asm ("rsi") = GATE_STACK_PAGES * page_size;
	register void (*r12)(uint32_t *) asm ("r12") = func;

	asm volatile (
		// replace stack
		"        mov     %%rsp, %%rdi                            \n"
		"        mov     %%r10, %%rsp                            \n"
		// unmap old stack (fails if stack pointer wasn't somewhere in the last frame)
		"        add     %%r11, %%rdi                            \n"
		"        not     %%r11                                   \n"
		"        and     %%r11, %%rdi                            \n"
		"        sub     %%rsi, %%rdi                            \n"
		"        mov     $"xstr(SYS_munmap)", %%eax              \n"
		"        syscall                                         \n"
		"        mov     $41, %%edi                              \n"
		"        test    %%rax, %%rax                            \n"
		"        jne     .exit                                   \n"
		// enable seccomp
		"        mov     $"xstr(SECCOMP_SET_MODE_STRICT)", %%edi \n"
		"        xor     %%rsi, %%rsi                            \n"
		"        xor     %%rdx, %%rdx                            \n"
		"        mov     $"xstr(SYS_seccomp)", %%eax             \n"
		"        syscall                                         \n"
		"        mov     $42, %%edi                              \n"
		"        test    %%rax, %%rax                            \n"
		"        jne     .exit                                   \n"
		// jump to the function with registers cleared
		"        mov     %%r12, %%rax                            \n"
		"        xor     %%rbx, %%rbx                            \n"
		"        xor     %%rcx, %%rcx                            \n"
		"        xor     %%rdx, %%rdx                            \n"
		"        xor     %%rbp, %%rbp                            \n"
		"        xor     %%rsi, %%rsi                            \n"
		"        xor     %%r8, %%r8                              \n"
		"        xor     %%r9, %%r9                              \n"
		"        xor     %%r10, %%r10                            \n"
		"        xor     %%r11, %%r11                            \n"
		"        xor     %%r12, %%r12                            \n"
		"        xor     %%r13, %%r13                            \n"
		"        xor     %%r14, %%r14                            \n"
		"        xor     %%r15, %%r15                            \n"
		"        jmp     *%%rax                                  \n"
		".exit:                                                  \n"
		"        mov     $"xstr(SYS_exit)", %%rax                \n"
		"        syscall                                         \n"
		"        hlt                                             \n"
		:
		: "r" (r10), "r" (r11), "r" (rsi), "r" (r12)
	);
}

struct stack_top {
	void *ptrs[2];
	uint64_t padding_1;
	void *tbss[1]; // TODO: caller must check size
	void *tcb;     //
	uint64_t padding_2;
} __attribute__ ((packed));

static int main(void)
{
	struct __attribute__ ((packed)) {
		uint64_t page_size;
		uint64_t text_addr;
		uint64_t text_size;
		uint64_t aligned_text_addr;
		uint64_t aligned_text_size;
		uint64_t rodata_addr;
		uint64_t rodata_size;
		uint64_t aligned_rodata_addr;
		uint64_t aligned_rodata_size;
		uint64_t data_addr;
		uint64_t data_size;
		uint64_t aligned_program_size;
		uint64_t indirect_funcs_size;
		uint64_t aligned_indirect_funcs_abi_size;
		uint64_t aligned_heap_size;
		uint64_t tbss_size;
		uint64_t unsafe_stack_ptr_offset;
		uint64_t indirect_call_check_addr;
		uint64_t args_addr;
		uint64_t start_addr;
	} info;

	if (read_full(&info, sizeof (info)) != 0)
		return 20;

	void *aligned_text_ptr = (void *) info.aligned_text_addr;

	if (sys_mmap(aligned_text_ptr, info.aligned_program_size, PROT_READ|PROT_WRITE, MAP_PRIVATE|MAP_ANONYMOUS|MAP_FIXED|MAP_NORESERVE, -1, 0) != aligned_text_ptr)
		return 21;

	void *text_ptr = (void *) info.text_addr;

	if (read_full(text_ptr, info.text_size) != 0)
		return 22;

	if (sys_mprotect(aligned_text_ptr, info.aligned_text_size, PROT_EXEC) != 0)
		return 23;

	if (info.rodata_size > 0) {
		void *rodata_ptr = (void *) info.rodata_addr;

		if (read_full(rodata_ptr, info.rodata_size) != 0)
			return 24;

		void *aligned_rodata_ptr = (void *) info.aligned_rodata_addr;

		if (sys_mprotect(aligned_rodata_ptr, info.aligned_rodata_size, PROT_READ) != 0)
			return 25;
	}

	if (info.data_size > 0) {
		void *data_ptr = (void *) info.data_addr;

		if (read_full(data_ptr, info.data_size) != 0)
			return 26;
	}

	indirect_func_array = sys_mmap(NULL, info.aligned_indirect_funcs_abi_size, PROT_READ|PROT_WRITE, MAP_PRIVATE|MAP_ANONYMOUS, -1, 0);
	if (indirect_func_array == MAP_FAILED)
		return 27;

	if ((uintptr_t) indirect_func_array < 0x100000000) // TODO: robustify
		return 28;

	if (read_full(indirect_func_array, info.indirect_funcs_size) != 0)
		return 29;

	indirect_func_count = info.indirect_funcs_size / sizeof (uint32_t);
	indirect_func_array[indirect_func_count++] = (uintptr_t) abi_exit;
	indirect_func_array[indirect_func_count++] = (uintptr_t) abi_recv;
	indirect_func_array[indirect_func_count++] = (uintptr_t) abi_send;

	if (sys_mprotect(indirect_func_array, info.aligned_indirect_funcs_abi_size, PROT_READ) != 0)
		return 30;

	void *heap_ptr = NULL;

	if (info.aligned_heap_size > 0) {
		heap_ptr = sys_mmap(NULL, info.aligned_heap_size, PROT_READ|PROT_WRITE, MAP_32BIT|MAP_PRIVATE|MAP_ANONYMOUS|MAP_NORESERVE, -1, 0);
		if (heap_ptr == MAP_FAILED)
			return 31;
	}

	size_t unsafe_stack_size = 8 * 1024 * 1024; // TODO

	void *unsafe_stack_start = sys_mmap(NULL, unsafe_stack_size, PROT_READ|PROT_WRITE, MAP_32BIT|MAP_GROWSDOWN|MAP_PRIVATE|MAP_ANONYMOUS|MAP_NORESERVE|MAP_STACK, -1, 0);
	if (unsafe_stack_start == MAP_FAILED)
		return 32;

	// TODO: guard page

	struct stack_top *top = unsafe_stack_start + unsafe_stack_size - sizeof (struct stack_top);

	void **tcb = &top->tcb;
	*tcb = tcb;

	if (sys_arch_prctl(ARCH_SET_FS, (unsigned long) tcb) != 0)
		return 33;

	top->tbss[info.unsafe_stack_ptr_offset / 8] = top; // XXX
	*(void **) info.indirect_call_check_addr = indirect_call_check;

	uint32_t *args = (void *) info.args_addr;

	args[GATE_ARG_ARGS_SIZE] = GATE_NUM_ARGS * sizeof (uint32_t);
	args[GATE_ARG_ABI_VERSION] = GATE_ABI_VERSION;
	args[GATE_ARG_HEAP_SIZE] = info.aligned_heap_size;
	args[GATE_ARG_HEAP_ADDR] = (uintptr_t) heap_ptr;
	args[GATE_ARG_OP_MAXSIZE] = 65536; // XXX
	args[GATE_ARG_FUNC_EXIT] = (uintptr_t) abi_exit;
	args[GATE_ARG_FUNC_RECV] = (uintptr_t) abi_recv;
	args[GATE_ARG_FUNC_SEND] = (uintptr_t) abi_send;

	size_t safe_stack_size = 8 * 1024 * 1024; // TODO

	// TODO: verify address randomization...
	void *safe_stack_start = sys_mmap(NULL, safe_stack_size, PROT_READ|PROT_WRITE, MAP_GROWSDOWN|MAP_PRIVATE|MAP_ANONYMOUS|MAP_NORESERVE|MAP_STACK, -1, 0);
	if (safe_stack_start == MAP_FAILED)
		return 34;

	if ((uintptr_t) safe_stack_start < 0x100000000) // TODO: robustify
		return 35;

	// TODO: guard page

	if (sys_mprotect((void *) GATE_BSS_START, GATE_BSS_SIZE, PROT_READ) != 0)
		return 36;

	if (sys_munmap((void *) GATE_GOT_START, GATE_GOT_SIZE) != 0)
		return 37;

	void *safe_stack_ptr = safe_stack_start + safe_stack_size - 2 * sizeof (void *); // aligned

	enter(info.page_size, safe_stack_ptr, (void *) info.start_addr);
}

__attribute__ ((noreturn))
void _start(void)
{
	sys_exit(main());
}
