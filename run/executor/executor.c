#include <stddef.h>
#include <stdlib.h>

#include <fcntl.h>
#include <unistd.h>
#include <sys/resource.h>
#include <sys/time.h>

#include "../defs.h"

extern char **environ;

static void xlimit(int resource, rlim_t rlim)
{
	struct rlimit buf;

	buf.rlim_cur = rlim;
	buf.rlim_max = rlim;

	if (setrlimit(resource, &buf) != 0)
		exit(4);
}

int main(int argc, char **argv)
{
	long page_size = sysconf(_SC_PAGESIZE);
	if (page_size < 0)
		return 3;

	xlimit(RLIMIT_AS, GATE_LIMIT_AS);
	xlimit(RLIMIT_CORE, 0);
	// RLIMIT_CPU
	xlimit(RLIMIT_DATA, 0);
	xlimit(RLIMIT_FSIZE, 0);
	xlimit(RLIMIT_MEMLOCK, 0);
	xlimit(RLIMIT_MSGQUEUE, 0);
	// RLIMIT_NICE
	xlimit(RLIMIT_NOFILE, GATE_LIMIT_FILENO);
	xlimit(RLIMIT_NPROC, 0);
	// RLIMIT_RTPRIO
	xlimit(RLIMIT_RTTIME, 0);
	// RLIMIT_SIGPENDING
	xlimit(RLIMIT_STACK, GATE_LOADER_STACK_PAGES * page_size);

	int flags = fcntl(GATE_LOADER_FD, F_GETFD);
	if (flags < 0)
		return 5;

	if (fcntl(GATE_LOADER_FD, F_SETFD, flags|FD_CLOEXEC) < 0)
		return 6;

	fexecve(GATE_LOADER_FD, argv, environ);
	return 7;
}
