// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

int __malloc_grow_memory(int n);

static inline long __malloc_morecore(long increment)
{
	int pages = __malloc_grow_memory(increment >> 16);
	if (pages >= 0)
		return (long) pages << 16;
	else
		return -1;
}
