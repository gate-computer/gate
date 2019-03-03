// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#define xstr(s) str(s)
#define str(s) #s

#ifndef GATE_SANDBOX
#error GATE_SANDBOX not defined
#endif

#if GATE_SANDBOX != 1
#warning sandbox is disabled
#endif

// Some of these values are also defined in runtime/runtime.go or
// internal/executable/runtime.go

#define GATE_MAX_PACKET_SIZE 65536

#define GATE_INPUT_FD 0   //                    exechild loader runtime
#define GATE_OUTPUT_FD 1  //                    exechild loader runtime
#define GATE_DEBUG_FD 2   // container executor exechild loader runtime
#define GATE_CONTROL_FD 3 // container executor exechild
#define GATE_LOADER_FD 4  // container executor exechild

#define GATE_MIN_HEAP_HIGH32 0x2aa9

#define GATE_LOADER_STACK_SIZE 12288LL // 3 pages

#define GATE_SIGNAL_STACK_RESERVE 8192
#define GATE_STACK_LIMIT_OFFSET (16 + GATE_SIGNAL_STACK_RESERVE + 128 + 16) // See wag/Stack.md

#define GATE_LIMIT_AS (GATE_LOADER_STACK_SIZE + /* */         \
		       0x1000LL +               /* loader */  \
		       0x1000LL +               /* runtime */ \
		       0x80000000LL +           /* text */    \
		       0x80000000LL +           /* stack */   \
		       0x1000LL +               /* globals */ \
		       0x80000000LL)            /* memory */
#define GATE_LIMIT_DATA 0x1000                  // Anonymous runtime mapping.
#define GATE_LIMIT_NOFILE 5                     // Input, output, debug, text, image.

#define GATE_TEXT_ADDR_RESUME 0x10 // Per wag object ABI.

#define GATE_MAGIC_NUMBER_1 0x8f3a
#define GATE_MAGIC_NUMBER_2 0x7e1c5d67
