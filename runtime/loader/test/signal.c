// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#define _GNU_SOURCE

#include <stdio.h>

#include <ucontext.h>

#ifdef __amd64__
int main()
{
	printf("ucontext: rbx offset: %ld\n", (void *) &((ucontext_t *) 0)->uc_mcontext.gregs[REG_RBX] - (void *) 0);

	printf("ucontext: rsp offset: %ld\n", (void *) &((ucontext_t *) 0)->uc_mcontext.gregs[REG_RSP] - (void *) 0);

	printf("ucontext: rip offset: %ld\n", (void *) &((ucontext_t *) 0)->uc_mcontext.gregs[REG_RIP] - (void *) 0);

	return 0;
}
#endif

#ifdef __aarch64__
int main()
{
	printf("ucontext: r28 offset: %ld\n", (void *) &((ucontext_t *) 0)->uc_mcontext.regs[28] - (void *) 0);

	printf("ucontext: r30 offset: %ld\n", (void *) &((ucontext_t *) 0)->uc_mcontext.regs[30] - (void *) 0);

	printf("ucontext: pc  offset: %ld\n", (void *) &((ucontext_t *) 0)->uc_mcontext.pc - (void *) 0);

	return 0;
}
#endif
