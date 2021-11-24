// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include "runtime.h"

.section .text

.globl	runtime_code_begin
.globl	runtime_code_end

.align	12 // 4096

runtime_code_begin:
	brk	0

.align	3 // 8

#include "../seccomp.S"

#ifdef __ANDROID__
#include "runtime-android.S"
#else
#include "runtime.S"
#endif

.align	3 // 8

runtime_code_end:
