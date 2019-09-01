// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#ifndef GATE_RUNTIME_EXECUTOR_EXECUTOR_H
#define GATE_RUNTIME_EXECUTOR_EXECUTOR_H

#include <stdint.h>

#define PACKED __attribute__((packed))

enum {
	EXEC_OP_CREATE,
	EXEC_OP_KILL,
	EXEC_OP_SUSPEND,
};

// See runtime/executor.go
struct exec_request {
	int16_t id;
	uint8_t op;
	uint8_t reserved[1];
} PACKED;

// See runtime/executor.go
struct exec_status {
	union {
		int32_t pid; // When buffered; before transformation.
		int16_t id;  // After transformation; when consumed.
	};
	int32_t status;
} PACKED;

extern bool no_namespaces;

#endif
