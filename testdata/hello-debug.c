// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <gate.h>

int main(void)
{
	gate_debug1(GATE_DEBUG_SEPARATOR "hello");
	gate_debug2(GATE_DEBUG_SEPARATOR, "world\n");
	return 0;
}
