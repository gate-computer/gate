// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <cstddef>
#include <new>

#include <gate.h>

void* operator new(size_t size)
{
	void* ptr = operator new(size, std::nothrow);
	if (ptr == nullptr) {
		gate_debug("out of memory\n");
		gate_exit(1);
	}
    return ptr;
}

void* operator new(size_t size, const std::nothrow_t&) noexcept
{
	if (size == 0)
		size = 1;
	return malloc(size);
}

void* operator new[](size_t size)
{
    return operator new(size);
}

void* operator new[](size_t size, const std::nothrow_t&) noexcept
{
    return operator new(size, std::nothrow);
}

void operator delete(void* ptr)
{
    operator delete(ptr, std::nothrow);
}

void operator delete(void* ptr, const std::nothrow_t&) noexcept
{
	free(ptr);
}

void operator delete[](void* ptr)
{
    operator delete(ptr);
}

void operator delete[](void* ptr, const std::nothrow_t&) noexcept
{
    operator delete(ptr, std::nothrow);
}
