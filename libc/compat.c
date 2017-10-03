// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <gate.h>

int *__errno_location()
{
	static int errno_storage;
	return &errno_storage;
}

void abort()
{
	gate_exit(1);
}
