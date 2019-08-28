// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#ifndef GATE_RUNTIME_EXECVEAT_H
#define GATE_RUNTIME_EXECVEAT_H

#include <sys/syscall.h>

#include "syscall.h"

static inline void sys_execveat(int dirfd, const char *pathname, char *const argv[], char *const envp[], int flags)
{
	syscall5(SYS_execveat, dirfd, (uintptr_t) pathname, (uintptr_t) argv, (uintptr_t) envp, flags);
}

#endif
