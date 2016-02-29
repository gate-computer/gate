#include <stddef.h>

#include <fcntl.h>
#include <unistd.h>
#include <sys/resource.h>
#include <sys/time.h>

#include "stack.h"

int main(int argc, char **argv)
{
	if (argc != 2)
		return 2;

	long page_size = sysconf(_SC_PAGESIZE);
	if (page_size < 0)
		return 3;

	struct rlimit rl;

	rl.rlim_cur = GATE_STACK_PAGES * page_size;
	rl.rlim_max = GATE_STACK_PAGES * page_size;
	if (setrlimit(RLIMIT_STACK, &rl) != 0)
		return 4;

	char *envp[] = { NULL };

	execve(argv[1], argv + 1, envp);
	return 5;
}
