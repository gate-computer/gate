// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

int __malloc_current_memory(void);
int __malloc_grow_memory(int n);

int __malloc_morecore(int increment)
{
	static int blocks;
	static int min_addr;
	static int addr;

	if (blocks == 0) {
		blocks = __malloc_current_memory();
		min_addr = blocks << 16;
		addr = min_addr;
	}

	int ret = addr;

	if (increment > 0) {
		int new_addr = addr + increment;
		if (new_addr < addr)
			return -1;

		int growth = (new_addr - (blocks << 16) + 0xffff) >> 16;
		if (growth > 0) {
			__malloc_grow_memory(growth);

			int new_blocks = blocks + growth;

			if (__malloc_current_memory() != new_blocks)
				return -1;

			blocks = new_blocks;
		}

		addr = new_addr;
	} else if (increment < 0) {
		int new_addr = addr - increment;
		if (new_addr < min_addr || new_addr > addr)
			return -1;

		for (char *p = (char *) new_addr; p != (char *) addr; p++)
			*p = 0;

		addr = new_addr;
	}

	return ret;
}
