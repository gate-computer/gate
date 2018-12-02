// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <sys/syscall.h>

static inline void sys_execveat(int dirfd, const char *pathname, char *const argv[], char *const envp[], int flags)
{
	register int rdi asm("rdi") = dirfd;
	register const char *rsi asm("rsi") = pathname;
	register char *const *rdx asm("rdx") = argv;
	register char *const *r10 asm("r10") = envp;
	register int r8 asm("r8") = flags;

	asm volatile(
		"syscall"
		:
		: "a"(SYS_execveat), "r"(rdi), "r"(rsi), "r"(rdx), "r"(r10), "r"(r8)
		: "cc", "rcx", "r11", "memory");
}
