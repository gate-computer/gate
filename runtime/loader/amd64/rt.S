// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

.intel_syntax noprefix

#include "runtime.hpp"

.section .text

.globl	rt_text_start
.globl	rt_text_end

.align	4096

rt_text_start:
	int3

.align	8

#include "seccomp.S"
#include "rt.gen.S"

.align	8

rt_text_end:
