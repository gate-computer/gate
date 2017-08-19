// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <sys/types.h>

struct cgroup_config {
	const char *title;
	const char *parent;
};

extern const char cgroup_backend[];

void init_cgroup(pid_t pid, const struct cgroup_config *config);
