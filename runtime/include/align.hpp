// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#pragma once

#include <cstddef>

static inline size_t align_size(size_t size, size_t alignment)
{
	size_t mask = alignment - 1;
	return (size + mask) & ~mask;
}
