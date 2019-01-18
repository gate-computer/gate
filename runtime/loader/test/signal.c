// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#define _GNU_SOURCE

#include <stdio.h>

#include <ucontext.h>

int main()
{
	printf("ucontext: rbx offset: %ld\n", (void *) &((ucontext_t *) 0)->uc_mcontext.gregs[REG_RBX] - (void *) 0);

	printf("ucontext: rsp offset: %ld\n", (void *) &((ucontext_t *) 0)->uc_mcontext.gregs[REG_RSP] - (void *) 0);

	printf("ucontext: rip offset: %ld\n", (void *) &((ucontext_t *) 0)->uc_mcontext.gregs[REG_RIP] - (void *) 0);

	return 0;
}
