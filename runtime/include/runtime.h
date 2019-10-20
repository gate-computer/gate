// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#ifndef GATE_RUNTIME_RUNTIME_H
#define GATE_RUNTIME_RUNTIME_H

#include <limits.h>

#define xstr(s) str(s)
#define str(s) #s

#define GATE_SANDBOX 1

#if GATE_SANDBOX != 1
#warning sandbox is disabled
#endif

// Some of these values are also defined in runtime/runtime.go or
// internal/executable/runtime.go

#define GATE_MAX_PACKET_SIZE 65536

#define GATE_INPUT_FD 0   //                    exechild loader runtime
#define GATE_OUTPUT_FD 1  //                    exechild loader runtime
#define GATE_DEBUG_FD 2   //                             loader runtime
#define GATE_CONTROL_FD 3 // container executor exechild
#define GATE_LOADER_FD 4  // container executor exechild
#define GATE_PROC_FD 6    // container executor

#define GATE_MIN_HEAP_HIGH32 0x2aa9

#define GATE_EXECUTOR_STACK_SIZE 65536LL // Depends on target architecture.
#define GATE_LOADER_STACK_SIZE 12288LL   // 3 pages

// See wag/Stack.md.
#define GATE_STACK_VARS_SIZE 64            // Variables at start of stack memory.
#define GATE_STACK_SIGNAL_SPACE (4832 * 2) // For simultaneous SIGSEGV and SIGXCPU handling.
#define GATE_STACK_USAGE_OFFSET (GATE_STACK_VARS_SIZE + GATE_STACK_SIGNAL_SPACE + 240)
#define GATE_STACK_LIMIT_OFFSET (GATE_STACK_USAGE_OFFSET + 8 + 8)

#define GATE_LIMIT_AS (GATE_LOADER_STACK_SIZE + /* */         \
		       0x1000LL +               /* loader */  \
		       0x1000LL +               /* runtime */ \
		       0x80000000LL +           /* text */    \
		       0x80000000LL +           /* stack */   \
		       0x1000LL +               /* globals */ \
		       0x80000000LL)            /* memory */

#if defined(__ANDROID__)
#define GATE_LIMIT_FSIZE 0
#define GATE_LIMIT_DATA 0xa64000 // Anonymous runtime mapping and something else?
#elif defined(__aarch64__)
#define GATE_LIMIT_FSIZE 44
#define GATE_LIMIT_DATA 0x2000 // Anonymous runtime mapping and something else?
#else
#define GATE_LIMIT_FSIZE 0
#define GATE_LIMIT_DATA 0x1000 // Anonymous runtime mapping.
#endif

#define GATE_LIMIT_NOFILE 5 // Input, output, debug, text, image.

#define GATE_TEXT_ADDR_RESUME 0x10 // Per wag object ABI.

#define GATE_MAGIC_NUMBER_1 0x19328f3a
#define GATE_MAGIC_NUMBER_2 0x975834d75125276c
#define GATE_STACK_MAGIC 0x7b53c485c17322fe

#endif
