// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include "runtime.hpp"

.section .text

.globl	rt_text_start
.globl	rt_text_end

.align	12 // 4096

rt_text_start:
	brk	0

.align	3 // 8

#include "seccomp.S"
#include "rt.gen.S"

.align	8 // 64 for memcpy.

rt_text_end:
