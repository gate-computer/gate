// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <cstdint>
#include <cstdio>

#include <ucontext.h>

ucontext_t* u = nullptr;

#ifdef __amd64__
uintptr_t reg_offset(unsigned reg_index)
{
	return reinterpret_cast<uintptr_t>(&u->uc_mcontext.gregs[reg_index]);
}

int main()
{
	printf("ucontext: rbx offset: %ld\n", reg_offset(REG_RBX));
	printf("ucontext: rsp offset: %ld\n", reg_offset(REG_RSP));
	printf("ucontext: rip offset: %ld\n", reg_offset(REG_RIP));
	return 0;
}
#endif

#ifdef __aarch64__
uintptr_t reg_offset(unsigned reg_index)
{
	return reinterpret_cast<uintptr_t>(&u->uc_mcontext.regs[reg_index]);
}

int main()
{
	printf("ucontext: r28 offset: %ld\n", reg_offset(28));
	printf("ucontext: r30 offset: %ld\n", reg_offset(30));
	printf("ucontext: pc  offset: %ld\n", reinterpret_cast<uintptr_t>(&u->uc_mcontext.pc));
	return 0;
}
#endif
