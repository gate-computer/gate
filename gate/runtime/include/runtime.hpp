// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#pragma once

#include <limits.h>

#define xstr(s) str(s)
#define str(s) #s

#define GATE_SANDBOX 1

#if GATE_SANDBOX != 1
#warning sandbox is disabled
#endif

// Some of these values are also defined in runtime/runtime.go or
// internal/container/common/common.go

#define GATE_MAX_PACKET_SIZE 65536

#define GATE_INPUT_FD 0   //                    exechild loader runtime
#define GATE_OUTPUT_FD 1  //                    exechild loader runtime
#define GATE_DEBUG_FD 2   //                             loader runtime
#define GATE_CONTROL_FD 3 // container executor exechild*
#define GATE_LOADER_FD 4  // container executor exechild
#define GATE_CGROUP_FD 6  // container executor exechild*
#define GATE_PROC_FD 7    // container executor exechild*

#define GATE_MIN_HEAP_HIGH32 0x2aa9

#define GATE_EXECUTOR_STACK_SIZE 65536LL // Depends on target architecture.
#define GATE_LOADER_STACK_SIZE 12288LL   // 3 pages

#if defined(__ANDROID__)
#define GATE_LIMIT_DATA 0xa64000 // Anonymous runtime mapping and something else?
#elif defined(__aarch64__)
#define GATE_LIMIT_DATA 0x2000 // Anonymous runtime mapping and something else?
#else
#define GATE_LIMIT_DATA 0x1000 // Anonymous runtime mapping.
#endif

#define GATE_LIMIT_NOFILE 5 // Input, output, debug, text, image.

#define GATE_TEXT_ADDR_RESUME 0x10 // Per wag object ABI.

#define GATE_MAGIC_NUMBER_1 0x19328f3a
#define GATE_MAGIC_NUMBER_2 0x975834d75125276c
#define GATE_STACK_MAGIC 0x7b53c485c17322fe
