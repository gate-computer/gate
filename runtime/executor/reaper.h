// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#ifndef GATE_RUNTIME_EXECUTOR_REAPER_H
#define GATE_RUNTIME_EXECUTOR_REAPER_H

#include <sys/types.h>

#include "map.h"

#define NORETURN __attribute__((noreturn))

struct params {
	struct pid_map pid_map;
	pid_t sentinel_pid;
	pid_t id_pids[ID_NUM] ALIGNED(CACHE_LINE_SIZE);
};

NORETURN void reaper(struct params *args);

#endif
