// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#define _GNU_SOURCE

#include "sentinel.h"

#include <signal.h>

#include <sys/prctl.h>
#include <unistd.h>

#include "errors.h"
#include "runtime.h"

// Close a file descriptor or die.
static void xclose(int fd)
{
	if (close(fd) != 0)
		_exit(ERR_SENTINEL_CLOSE);
}

void sentinel(void)
{
	if (prctl(PR_SET_PDEATHSIG, SIGKILL) != 0)
		_exit(ERR_SENTINEL_PRCTL_PDEATHSIG);

	xclose(GATE_CONTROL_FD);
	xclose(GATE_LOADER_FD);
	xclose(GATE_PROC_FD);

	sigset_t mask;
	sigfillset(&mask);
	sigdelset(&mask, SIGBUS);
	sigdelset(&mask, SIGFPE);
	sigdelset(&mask, SIGILL);
	sigdelset(&mask, SIGSEGV);
	sigdelset(&mask, SIGTERM); // Sent by executor.
	sigsuspend(&mask);
	_exit(ERR_SENTINEL_SIGSUSPEND);
}
