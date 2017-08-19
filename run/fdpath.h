// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include "defs.h"

static inline char *get_fd_path(int fd, char **envp)
{
	if (envp[0] == NULL || envp[1] != NULL) // Exactly one variable
		return NULL;

	char *path = envp[0];
	envp[0] = NULL;

	// Avoid strlen
	size_t len = 0;
	while (path[len])
		len++;

	if (len != GATE_FD_PATH_LEN)
		return NULL;

	path[len - 1] = '0' + fd; // This assumes that all fds are < 10
	return path;
}
