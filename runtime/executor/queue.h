// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#ifndef GATE_RUNTIME_EXECUTOR_QUEUE_H
#define GATE_RUNTIME_EXECUTOR_QUEUE_H

#define QUEUE_BUFLEN 256

static inline unsigned int queue_wrap(unsigned int i)
{
	return i & (QUEUE_BUFLEN - 1);
}

#endif
