// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

.intel_syntax noprefix

#include "runtime.hpp"

.section .text

.globl	runtime_code_begin
.globl	runtime_code_end

.align	4096

runtime_code_begin:
	int3

.align	8

#include "../seccomp.S"

#ifdef __ANDROID__
#include "runtime-android.S"
#else
#include "runtime.S"
#endif

.align	8

runtime_code_end:
