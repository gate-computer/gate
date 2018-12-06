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

#define GATE_MAX_PACKET_SIZE 65536

#define GATE_INPUT_FD 0   //                    exechild loader runtime
#define GATE_OUTPUT_FD 1  //                    exechild loader runtime
#define GATE_DEBUG_FD 2   //                    exechild loader runtime
#define GATE_CONTROL_FD 3 // container executor exechild
#define GATE_IMAGE_FD 3   //                    exechild loader
#define GATE_LOADER_FD 4  // container executor exechild

#define GATE_LOADER_STACK_SIZE 12288LL // 3 pages

#define GATE_SIGNAL_STACK_RESERVE 8192
#define GATE_SIGNAL_STACK_SUSPEND_REG_OFFSET 136

#define GATE_LIMIT_AS (GATE_LOADER_STACK_SIZE + /* */         \
		       0x1000LL +               /* loader */  \
		       0x1000LL +               /* runtime */ \
		       0x80000000LL +           /* text */    \
		       0x80000000LL +           /* stack */   \
		       0x1000LL +               /* globals */ \
		       0x80000000LL)            /* memory */
#define GATE_LIMIT_DATA 0x1000                  // Anonymous runtime mapping.

#define GATE_MAGIC_NUMBER_1 0x53058f3a
#define GATE_MAGIC_NUMBER_2 0x7e1c5d67