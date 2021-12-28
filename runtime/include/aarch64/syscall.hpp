// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#ifndef GATE_RUNTIME_SYSCALL_HPP
#define GATE_RUNTIME_SYSCALL_HPP

#include <cstddef>

static inline intptr_t syscall1(int nr, uintptr_t a1)
{
	register uintptr_t x0 asm("x0") = a1;
	register int x8 asm("x8") = nr;

	asm volatile(
		"svc 0"
		: "+r"(x0)
		: "r"(x0), "r"(x8)
		: "cc", "memory");

	return x0;
}

static inline intptr_t syscall2(int nr, uintptr_t a1, uintptr_t a2)
{
	register uintptr_t x0 asm("x0") = a1;
	register uintptr_t x1 asm("x1") = a2;
	register int x8 asm("x8") = nr;

	asm volatile(
		"svc 0"
		: "+r"(x0)
		: "r"(x0), "r"(x1), "r"(x8)
		: "cc", "memory");

	return x0;
}

static inline intptr_t syscall3(int nr, uintptr_t a1, uintptr_t a2, uintptr_t a3)
{
	register uintptr_t x0 asm("x0") = a1;
	register uintptr_t x1 asm("x1") = a2;
	register uintptr_t x2 asm("x2") = a3;
	register int x8 asm("x8") = nr;

	asm volatile(
		"svc 0"
		: "+r"(x0)
		: "r"(x0), "r"(x1), "r"(x2), "r"(x8)
		: "cc", "memory");

	return x0;
}

static inline intptr_t syscall4(int nr, uintptr_t a1, uintptr_t a2, uintptr_t a3, uintptr_t a4)
{
	register uintptr_t x0 asm("x0") = a1;
	register uintptr_t x1 asm("x1") = a2;
	register uintptr_t x2 asm("x2") = a3;
	register uintptr_t x3 asm("x3") = a4;
	register int x8 asm("x8") = nr;

	asm volatile(
		"svc 0"
		: "+r"(x0)
		: "r"(x0), "r"(x1), "r"(x2), "r"(x3), "r"(x8)
		: "cc", "memory");

	return x0;
}

static inline intptr_t syscall5(int nr, uintptr_t a1, uintptr_t a2, uintptr_t a3, uintptr_t a4, uintptr_t a5)
{
	register uintptr_t x0 asm("x0") = a1;
	register uintptr_t x1 asm("x1") = a2;
	register uintptr_t x2 asm("x2") = a3;
	register uintptr_t x3 asm("x3") = a4;
	register uintptr_t x4 asm("x4") = a5;
	register int x8 asm("x8") = nr;

	asm volatile(
		"svc 0"
		: "+r"(x0)
		: "r"(x0), "r"(x1), "r"(x2), "r"(x3), "r"(x4), "r"(x8)
		: "cc", "memory");

	return x0;
}

static inline intptr_t syscall6(int nr, uintptr_t a1, uintptr_t a2, uintptr_t a3, uintptr_t a4, uintptr_t a5, uintptr_t a6)
{
	register uintptr_t x0 asm("x0") = a1;
	register uintptr_t x1 asm("x1") = a2;
	register uintptr_t x2 asm("x2") = a3;
	register uintptr_t x3 asm("x3") = a4;
	register uintptr_t x4 asm("x4") = a5;
	register uintptr_t x5 asm("x5") = a6;
	register int x8 asm("x8") = nr;

	asm volatile(
		"svc 0"
		: "+r"(x0)
		: "r"(x0), "r"(x1), "r"(x2), "r"(x3), "r"(x4), "r"(x5), "r"(x8)
		: "cc", "memory");

	return x0;
}

#endif
