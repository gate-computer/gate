// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <gate.h>

int main(void);
void __wasm_call_ctors(void);

void _start(void)
{
	__wasm_call_ctors();
	__gate_exit(main());
}
