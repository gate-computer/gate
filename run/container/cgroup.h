#include <sys/types.h>

struct cgroup_config {
	const char *title;
	const char *parent;
};

extern const char cgroup_backend[];

void init_cgroup(pid_t pid, const struct cgroup_config *config);
