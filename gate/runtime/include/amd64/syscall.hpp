// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#pragma once

#include <cstddef>

namespace runtime {

intptr_t syscall(int nr, uintptr_t a1)
{
	intptr_t retval;
	register uintptr_t rdi asm("rdi") = a1;

	asm volatile(
		"syscall"
		: "=a"(retval)
		: "a"(nr), "r"(rdi)
		: "cc", "rcx", "r11", "memory");

	return retval;
}

intptr_t syscall(int nr, uintptr_t a1, uintptr_t a2)
{
	intptr_t retval;
	register uintptr_t rdi asm("rdi") = a1;
	register uintptr_t rsi asm("rsi") = a2;

	asm volatile(
		"syscall"
		: "=a"(retval)
		: "a"(nr), "r"(rdi), "r"(rsi)
		: "cc", "rcx", "r11", "memory");

	return retval;
}

intptr_t syscall(int nr, uintptr_t a1, uintptr_t a2, uintptr_t a3)
{
	intptr_t retval;
	register uintptr_t rdi asm("rdi") = a1;
	register uintptr_t rsi asm("rsi") = a2;
	register uintptr_t rdx asm("rdx") = a3;

	asm volatile(
		"syscall"
		: "=a"(retval)
		: "a"(nr), "r"(rdi), "r"(rsi), "r"(rdx)
		: "cc", "rcx", "r11", "memory");

	return retval;
}

intptr_t syscall(int nr, uintptr_t a1, uintptr_t a2, uintptr_t a3, uintptr_t a4)
{
	intptr_t retval;
	register uintptr_t rdi asm("rdi") = a1;
	register uintptr_t rsi asm("rsi") = a2;
	register uintptr_t rdx asm("rdx") = a3;
	register uintptr_t r10 asm("r10") = a4;

	asm volatile(
		"syscall"
		: "=a"(retval)
		: "a"(nr), "r"(rdi), "r"(rsi), "r"(rdx), "r"(r10)
		: "cc", "rcx", "r11", "memory");

	return retval;
}

intptr_t syscall(int nr, uintptr_t a1, uintptr_t a2, uintptr_t a3, uintptr_t a4, uintptr_t a5)
{
	intptr_t retval;
	register uintptr_t rdi asm("rdi") = a1;
	register uintptr_t rsi asm("rsi") = a2;
	register uintptr_t rdx asm("rdx") = a3;
	register uintptr_t r10 asm("r10") = a4;
	register uintptr_t r8 asm("r8") = a5;

	asm volatile(
		"syscall"
		: "=a"(retval)
		: "a"(nr), "r"(rdi), "r"(rsi), "r"(rdx), "r"(r10), "r"(r8)
		: "cc", "rcx", "r11", "memory");

	return retval;
}

intptr_t syscall(int nr, uintptr_t a1, uintptr_t a2, uintptr_t a3, uintptr_t a4, uintptr_t a5, uintptr_t a6)
{
	intptr_t retval;
	register uintptr_t rdi asm("rdi") = a1;
	register uintptr_t rsi asm("rsi") = a2;
	register uintptr_t rdx asm("rdx") = a3;
	register uintptr_t r10 asm("r10") = a4;
	register uintptr_t r8 asm("r8") = a5;
	register uintptr_t r9 asm("r9") = a6;

	asm volatile(
		"syscall"
		: "=a"(retval)
		: "a"(nr), "r"(rdi), "r"(rsi), "r"(rdx), "r"(r10), "r"(r8), "r"(r9)
		: "cc", "rcx", "r11", "memory");

	return retval;
}

} // namespace runtime
