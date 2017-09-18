// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// some of these are also defined in defs.go, run.js, work.js and loader/tests/stack.py

#define GATE_RODATA_ADDR 0x10000

#define GATE_LOADER_STACK_SIZE 12288 // 3 regular pages

#define GATE_CONTROL_FD  3 // container, executor
#define GATE_LOADER_FD   4 // container, executor

#define GATE_INPUT_FD    0 // loader, runtime
#define GATE_OUTPUT_FD   1 // loader, runtime
#define GATE_DEBUG_FD    2 // loader, runtime
#define GATE_MAPS_FD     3 // loader
#define GATE_WAKEUP_FD   4 // Imaginary

#define GATE_FD_PATH_LEN (sizeof ("/.XXXXXXXXXXXXXXXX/self/fd/X") - 1)

#define GATE_SIGNAL_STACK_RESERVE   0x600 // TODO
#define GATE_SIGNAL_STACK_R9_OFFSET 56

#define GATE_ABI_VERSION     0
#define GATE_MAX_PACKET_SIZE 0x10000

#define GATE_PACKET_FLAG_TRAP 0x8000

#define GATE_LIMIT_AS (0x80000000LL + /* rodata */ \
                       0x1000LL     + /* loader .runtime section */ \
                       0x80000000LL + /* text */ \
                       0x80000000LL + /* globals + memory */ \
                       0x80000000LL)  /* stack */

#define GATE_LIMIT_NOFILE GATE_WAKEUP_FD

#define GATE_MAGIC_NUMBER 0x7e1c5d67
