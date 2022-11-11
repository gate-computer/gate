// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The memory access instructions generated by wag use a zero-extended register
// as variable offset, with a 31-bit constant offset.
#define MEMORY_ADDRESS_RANGE (0x100000000ULL + 0x80000000ULL)

namespace runtime::loader {

bool strcmp_clock_gettime(char const* name)
{
	if (strlen(name) != 20)
		return false;

	if (reinterpret_cast<uint64_t const*>(name)[0] != 0x635f6f7364765f5fULL) // Little-endian "__vdso_c"
		return false;

	if (reinterpret_cast<uint64_t const*>(name)[1] != 0x7465675f6b636f6cULL) // Little-endian "lock_get"
		return false;

	if (reinterpret_cast<uint32_t const*>(name)[4] != 0x656d6974UL) // Little-endian "time"
		return false;

	return true;
}

NORETURN void enter_rt(void* stack_ptr, uintptr_t stack_limit UNUSED)
{
	register auto rsp asm("rsp") = stack_ptr;

	asm volatile("jmp start_rt" :: "r"(rsp));
	__builtin_unreachable();
}

} // namespace runtime::loader