#include <stddef.h>

#include <fcntl.h>
#include <unistd.h>
#include <sys/resource.h>
#include <sys/time.h>

#include "../defs.h"

extern char **environ;

int main(int argc, char **argv)
{
	long page_size = sysconf(_SC_PAGESIZE);
	if (page_size < 0)
		return 3;

	struct rlimit rl;

	rl.rlim_cur = GATE_STACK_PAGES * page_size;
	rl.rlim_max = GATE_STACK_PAGES * page_size;
	if (setrlimit(RLIMIT_STACK, &rl) != 0)
		return 4;

	int flags = fcntl(GATE_LOADER_FD, F_GETFD);
	if (flags < 0)
		return 5;

	if (fcntl(GATE_LOADER_FD, F_SETFD, flags|FD_CLOEXEC) < 0)
		return 6;

	fexecve(GATE_LOADER_FD, argv, environ);
	return 7;
}
