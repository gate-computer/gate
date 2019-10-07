// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#ifndef GATE_RUNTIME_DEBUG_H
#define GATE_RUNTIME_DEBUG_H

#define GATE_RUNTIME_DEBUG 0

#if GATE_RUNTIME_DEBUG
#include <stdio.h>
#define debugf(...)                           \
	do {                                  \
		fprintf(stderr, __VA_ARGS__); \
		fprintf(stderr, "\n");        \
	} while (0)
#else
#define debugf(...) \
	do {        \
	} while (0)
#endif

#endif
