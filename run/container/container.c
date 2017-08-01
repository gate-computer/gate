#define _GNU_SOURCE

#include <errno.h>
#include <limits.h>
#include <signal.h>
#include <stddef.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include <fcntl.h>
#include <grp.h>
#include <libgen.h>
#include <sched.h>
#include <sys/mount.h>
#include <sys/prctl.h>
#include <sys/resource.h>
#include <sys/socket.h>
#include <sys/stat.h>
#include <sys/syscall.h>
#include <sys/time.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <unistd.h>

#include <sys/capability.h>

#include "../defs.h"
#include "cgroup.h"

#define EXECUTOR_FILENAME "executor"
#define LOADER_FILENAME   "loader"

struct identity {
	uid_t uid;
	gid_t gid;
};

static int parent_main(pid_t child_pid);
static int child_main(void *);

static int pivot_root(const char *new_root, const char *put_old)
{
	long ret = syscall(SYS_pivot_root, new_root, put_old);
	if (ret < 0) {
		errno = -ret;
		return -1;
	}

	return 0;
}

// Print an error message and die.
static void xerror(const char *s)
{
	perror(s);
	exit(1);
}

// Close a file descriptor or die.
static void xclose(int fd)
{
	if (close(fd) != 0)
		xerror("close");
}

// Set a resource limit or die.
static void xlimit(int resource, rlim_t rlim)
{
	struct rlimit buf;

	buf.rlim_cur = rlim;
	buf.rlim_max = rlim;

	if (setrlimit(resource, &buf) != 0)
		xerror("setrlimit");
}

// Duplicate a file descriptor or die.
static void xdup2(int oldfd, int newfd)
{
	if (dup2(oldfd, newfd) != newfd)
		xerror("dup2");
}

// Make sure that this process doesn't outlive its parent.
static void xdeathsigkill()
{
	if (prctl(PR_SET_PDEATHSIG, SIGKILL) != 0)
		xerror("PR_SET_PDEATHSIG");

	// parent died already? (assuming it wasn't the init process)
	if (getppid() == 1)
		raise(SIGKILL);
}

// Clear all process capabilities or die.
static void xcapclear()
{
	cap_t p = cap_init();
	if (p == NULL)
		xerror("cap_init");

	cap_clear(p);

	if (cap_set_proc(p) != 0)
		xerror("cap_set_proc");

	cap_free(p);
}

// Convert a base 10 string to an unsigned integer or die.
static unsigned int xatoui(const char *s)
{
	if (*s == '\0') {
		errno = ERANGE;
		xerror(s);
	}

	char *end;
	unsigned long n = strtoul(s, &end, 10);
	if (*end != '\0')
		xerror(s);

	if (n >= UINT_MAX) {
		errno = ERANGE;
		xerror(s);
	}

	return n;
}

// Configure given process's uid_map or gid_map, or die.
static void xwritemap(pid_t pid, const char *filename, const char *data, int datalen)
{
	char path[32];
	int pathlen = snprintf(path, sizeof (path), "/proc/%u/%s", pid, filename);
	if (pathlen < 0 || pathlen >= (int) sizeof (path))
		xerror("snprintf uid_map/gid_map path");

	int fd = open(path, O_WRONLY);
	if (fd < 0)
		xerror(path);

	if (write(fd, data, datalen) != datalen)
		xerror(path);

	xclose(fd);
}

// Open a file, or die.
static int xopen(const char *dir, const char *file, int flags)
{
	size_t pathsize = strlen(dir) + 1 + strlen(file) + 1;
	char path[pathsize];

	strcpy(path, dir);
	strcat(path, "/");
	strcat(path, file);

	int fd = open(path, flags, 0);
	if (fd < 0)
		xerror(path);

	return fd;
}

// Open loader and executor binaries, or die.  Only executor fd is returned.
// The hard-coded GATE_LOADER_FD is valid after this.
static int xopenbinaries()
{
	// lstat'ing a symlink in /proc doesn't yield target path length :(
	char linkbuf[PATH_MAX];
	ssize_t linklen = readlink("/proc/self/exe", linkbuf, sizeof (linkbuf));
	if (linklen <= 0 || linklen >= (ssize_t) sizeof (linkbuf))
		xerror("readlink /proc/self/exe");
	linkbuf[linklen] = '\0';

	const char *dir = dirname(linkbuf); // linkbuf is unusable after this

	int loader_fd = xopen(dir, LOADER_FILENAME, O_RDONLY);
	if (loader_fd != GATE_LOADER_FD) {
		fprintf(stderr, "wrong number of open files\n");
		exit(1);
	}

	return xopen(dir, EXECUTOR_FILENAME, O_RDONLY|O_CLOEXEC);
}

// Fork with clone flags, or die.
static int xclone(int (*fn)(void *), int flags)
{
	// The function pointer and its argument (8 bytes each) are stored on
	// the stack before the address space is cloned.  Also provide 128
	// bytes for the red zone, just in case.  After the address space is
	// cloned, the child can use the same stack addresses as the parent, so
	// this staging area doesn't have to cover user code.
	union {
		char stack[128 + 8 + 8];
		__int128 alignment;
	} clobbered;

	void *stack_top = clobbered.stack + sizeof (clobbered.stack);

	int pid = clone(fn, stack_top, flags, NULL);
	if (pid <= 0)
		xerror("clone");

	return pid;
}

static struct identity identities[2];
static gid_t supplementary_gid;
static struct cgroup_config cgroup_config;
static int syncpipe[2];

int main(int argc, char **argv)
{
	if (argc == 2 && strcmp(argv[1], "--cgroup-backend") == 0) {
		puts(cgroup_backend);
		return 0;
	}

	if (argc != 8) {
		fprintf(stderr, "%s: argc != 8\n", argv[0]);
		return 1;
	}

	identities[0].uid = xatoui(argv[1]);
	identities[0].gid = xatoui(argv[2]);
	identities[1].uid = xatoui(argv[3]);
	identities[1].gid = xatoui(argv[4]);
	supplementary_gid = xatoui(argv[5]);
	cgroup_config.title = argv[6];
	cgroup_config.parent = argv[7];

	umask(0777);

	xlimit(RLIMIT_FSIZE, 0);
	xlimit(RLIMIT_MEMLOCK, 0);
	xlimit(RLIMIT_MSGQUEUE, 0);
	xlimit(RLIMIT_RTPRIO, 0);
	xlimit(RLIMIT_RTTIME, 0);
	xlimit(RLIMIT_SIGPENDING, 0); // applies only to sigqueue

	if (setgroups(0, NULL) != 0)
		xerror("setgroups to empty");

	if (prctl(PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0) != 0)
		xerror("PR_SET_NO_NEW_PRIVS");

	if (pipe2(syncpipe, O_CLOEXEC) != 0)
		xerror("pipe");

	pid_t child_pid = xclone(child_main, CLONE_NEWCGROUP | CLONE_NEWIPC | CLONE_NEWNET | CLONE_NEWNS | CLONE_NEWPID | CLONE_NEWUSER | CLONE_NEWUTS | SIGCHLD);

	return parent_main(child_pid);
}

static int parent_main(pid_t child_pid)
{
	xclose(GATE_CONTROL_FD);
	xclose(syncpipe[0]);

	char *map;
	int maplen;

	maplen = asprintf(&map, "1 %u 1\n2 %u 1\n3 %u 1\n", getuid(), identities[0].uid, identities[1].uid);
	if (maplen < 0)
		xerror("asprintf uid_map");
	xwritemap(child_pid, "uid_map", map, maplen);
	free(map);

	maplen = asprintf(&map, "1 %u 1\n2 %u 1\n3 %u 1\n4 %u 1\n", getgid(), identities[0].gid, identities[1].gid, supplementary_gid);
	if (maplen < 0)
		xerror("asprintf gid_map");
	xwritemap(child_pid, "gid_map", map, maplen);
	free(map);

	// user namespace configured

	init_cgroup(child_pid, &cgroup_config);

	xcapclear();

	// cgroup configured

	xclose(syncpipe[1]); // wake child up

	while (1) {
		int status;
		pid_t pid = wait(&status);
		if (pid < 0) {
			if (errno == EINTR)
				continue;

			xerror("wait");
		}

		if (pid != child_pid)
			continue;

		if (WIFSTOPPED(status) || WIFCONTINUED(status))
			continue;

		if (WIFEXITED(status))
			return WEXITSTATUS(status);

		if (WIFSIGNALED(status))
			raise(WTERMSIG(status));

		fprintf(stderr, "wait: unknown status: %d\n", status);
		return 1;
	}
}

static int child_main(void *dummy_arg)
{
	xdeathsigkill();

	xclose(syncpipe[1]);

	while (1) {
		char buf[1];
		ssize_t len = read(syncpipe[0], buf, sizeof (buf));
		if (len < 0) {
			if (errno == EINTR)
				continue;

			xerror("read from sync pipe");
		}

		xclose(syncpipe[0]);
		break;
	}

	// user namespace and cgroup have been configured by parent

	int executor_fd = xopenbinaries();

	// supplementary group
	const gid_t groups[1] = {4};
	if (setgroups(1, groups) != 0)
		xerror("setgroups");

	// bootstrap identity
	if (setreuid(2, 2) != 0)
		xerror("setuid for bootstrapping");
	if (setregid(2, 2) != 0)
		xerror("setgid for bootstrapping");

	if (sethostname("", 0) != 0)
		xerror("sethostname to empty");

	if (setdomainname("", 0) != 0)
		xerror("setdomainname to empty");

	if (mount("", "/", "", MS_PRIVATE|MS_REC, NULL) != 0)
		xerror("remount old root as private recursively");

	int mount_options = MS_NODEV|MS_NOEXEC|MS_NOSUID;

	// abuse /tmp as staging area for new root
	if (mount("tmpfs", "/tmp", "tmpfs", mount_options, "mode=0111,nr_blocks=1,nr_inodes=3") != 0)
		xerror("mount small tmpfs at /tmp");

	if (mkdir("/tmp/proc", 0) != 0)
		xerror("mkdir inside small tmpfs");

	if (mount("proc", "/tmp/proc", "proc", mount_options, NULL) != 0)
		xerror("mount proc inside would-be root");

	if (mkdir("/tmp/pivot", 0) != 0)
		xerror("mkdir inside small tmpfs");

	if (pivot_root("/tmp", "/tmp/pivot") != 0)
		xerror("pivot root");

	if (chdir("/") != 0)
		xerror("chdir to new root");

	if (umount2("/pivot", MNT_DETACH) != 0)
		xerror("umount old root");

	if (rmdir("/pivot") != 0)
		xerror("rmdir old root");

	mount_options |= MS_RDONLY;

	if (mount("", "/", "", MS_REMOUNT|mount_options, NULL) != 0)
		xerror("remount new root as read-only");

	// execution identity
	if (setreuid(3, 3) != 0)
		xerror("setuid for execution");
	if (setregid(3, 3) != 0)
		xerror("setgid for execution");

	xcapclear();

	// enable scheduler's autogroup feature
	if (setsid() < 0)
		xerror("setsid");

	long pagesize = sysconf(_SC_PAGESIZE);
	if (pagesize < 0)
		xerror("sysconf _SC_PAGESIZE");

	xlimit(RLIMIT_AS, GATE_LIMIT_AS);
	xlimit(RLIMIT_CORE, 0);
	xlimit(RLIMIT_STACK, GATE_LOADER_STACK_PAGES * pagesize);

	xdup2(STDOUT_FILENO, STDERR_FILENO); // /dev/null

	char *empty[] = {NULL};
	fexecve(executor_fd, empty, empty);
	// stderr doesn't work anymore
	return 2;
}
