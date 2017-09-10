// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

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
#include <linux/random.h>
#include <sched.h>
#include <spawn.h>
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

#define NEWUIDMAP_PATH "/usr/bin/newuidmap"
#define NEWGIDMAP_PATH "/usr/bin/newgidmap"

#define EXECUTOR_FILENAME "executor"
#define LOADER_FILENAME   "loader"

extern char **environ;

struct cred {
	uid_t uid;
	gid_t gid;
};

static int parent_main(pid_t child_pid);
static int child_main(void *);

// Print an error message and die.
static void xerror(const char *s)
{
	perror(s);
	exit(1);
}

// Get random bytes or die.
static void xgetentropy(void *buf, size_t buflen)
{
	for (size_t got = 0; got < buflen; ) {
		ssize_t len = syscall(SYS_getrandom, buf + got, buflen - got, 0);
		if (len <= 0) {
			if (len == 0)
				errno = EIO;

			if (errno == EINTR)
				continue;

			xerror("getentropy");
		}

		got += len;
	}
}

// Close a file descriptor or die.
static void xclose(int fd)
{
	if (close(fd) != 0)
		xerror("close");
}

// Duplicate a file descriptor or die.
static void xdup2(int oldfd, int newfd)
{
	if (dup2(oldfd, newfd) != newfd)
		xerror("dup2");
}

// Read from blocking file descriptor until EOF, or die.
static void xread_until_eof(int fd)
{
	while (1) {
		char buf[1];
		ssize_t len = read(fd, buf, sizeof (buf));
		if (len <= 0) {
			if (len == 0)
				return;

			if (errno == EINTR)
				continue;

			xerror("read");
		}
	}
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

// Make sure that this process doesn't outlive its parent.
static void xset_pdeathsig(int signum)
{
	if (prctl(PR_SET_PDEATHSIG, signum) != 0)
		xerror("PR_SET_PDEATHSIG");

	// parent died already? (assuming it wasn't the init process)
	if (getppid() == 1)
		raise(signum);
}

// Clear all process capabilities or die.
static void xclear_caps()
{
	cap_t p = cap_init();
	if (p == NULL)
		xerror("cap_init");

	cap_clear(p);

	if (cap_set_proc(p) != 0)
		xerror("cap_set_proc");

	cap_free(p);
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

// Change the root filesystem or die.
static void xpivot_root(const char *new_root, const char *put_old)
{
	long ret = syscall(SYS_pivot_root, new_root, put_old);
	if (ret < 0) {
		errno = -ret;
		xerror("pivot root");
	}
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

// Convert an unsigned integer to a base 10 string or die.  The returned string
// must be freed.
static char *xuitoa(unsigned int i)
{
	char *s;

	if (asprintf(&s, "%u", i) < 0)
		xerror("asprintf");

	return s;
}

// Configure given process's uid_map or gid_map, or die.
static void xwrite_id_map(pid_t target, const char *prog, unsigned int current, unsigned int container, unsigned int executor)
{
	char *target_str = xuitoa(target);
	char *current_str = xuitoa(current);
	char *container_str = xuitoa(container);
	char *executor_str = xuitoa(executor);

	char *args[] = {
		(char *) prog,
		target_str,
		// inside, outside, count
		"1", current_str,   "1",
		"2", container_str, "1",
		"3", executor_str,  "1",
		NULL,
	};

	pid_t prog_pid;
	errno = posix_spawn(&prog_pid, prog, NULL, NULL, args, environ);
	if (errno != 0)
		xerror(prog);

	free(executor_str);
	free(container_str);
	free(current_str);
	free(target_str);

	while (1) {
		int status;
		pid_t retval = waitpid(prog_pid, &status, 0);
		if (retval < 0) {
			if (errno == EINTR)
				continue;

			xerror("waitpid");
		}

		if (WIFEXITED(status)) {
			if (WEXITSTATUS(status) == 0)
				break;
		} else {
			fprintf(stderr, "%s terminated with status %d\n", prog, status);
		}

		exit(1);
	}
}

// Configure given process's uid_map or die.
static void xwrite_uid_map(pid_t pid, uid_t container, uid_t executor)
{
	xwrite_id_map(pid, NEWUIDMAP_PATH, getuid(), container, executor);
}

// Configure given process's gid_map or die.
static void xwrite_gid_map(pid_t pid, gid_t container, gid_t executor)
{
	xwrite_id_map(pid, NEWGIDMAP_PATH, getgid(), container, executor);
}

// Open a file in a directory, or die.
static int xopen_dir_file(const char *dir, const char *file, int flags)
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
static int xopen_executor_and_loader()
{
	// lstat'ing a symlink in /proc doesn't yield target path length :(
	char linkbuf[PATH_MAX];
	ssize_t linklen = readlink("/proc/self/exe", linkbuf, sizeof (linkbuf));
	if (linklen <= 0 || linklen >= (ssize_t) sizeof (linkbuf))
		xerror("readlink /proc/self/exe");
	linkbuf[linklen] = '\0';

	const char *dir = dirname(linkbuf); // linkbuf is unusable after this

	int loader_fd = xopen_dir_file(dir, LOADER_FILENAME, O_RDONLY);
	if (loader_fd != GATE_LOADER_FD) {
		fprintf(stderr, "wrong number of open files\n");
		exit(1);
	}

	return xopen_dir_file(dir, EXECUTOR_FILENAME, O_RDONLY|O_CLOEXEC);
}

// Wait for the child process, or die.  The return code is returned.
static int wait_for_child(pid_t child_pid)
{
	while (1) {
		int status;
		pid_t pid = wait(&status);
		if (pid < 0) {
			if (errno == EINTR)
				continue;

			xerror("wait");
		}

		if (pid != child_pid) {
			fprintf(stderr, "unknown child process %d terminated with status %d\n", pid, status);
			exit(1);
		}

		if (WIFSTOPPED(status)) {
			fprintf(stderr, "executor process %d received SIGSTOP\n", pid);
			continue;
		}

		if (WIFCONTINUED(status)) {
			fprintf(stderr, "executor process %d received SIGCONT\n", pid);
			continue;
		}

		if (WIFEXITED(status))
			return WEXITSTATUS(status);

		if (WIFSIGNALED(status))
			return 128 + WTERMSIG(status);

		fprintf(stderr, "wait: unknown status: %d\n", status);
		exit(1);
	}
}

static struct cred container_cred;
static struct cred executor_cred;
static struct cgroup_config cgroup_config;
static int syncpipe[2];

int main(int argc, char **argv)
{
	if (argc == 2 && strcmp(argv[1], "--cgroup-backend") == 0) {
		puts(cgroup_backend);
		return 0;
	}

	if (argc != 7) {
		fprintf(stderr, "%s: argc != 8\n", argv[0]);
		return 1;
	}

	container_cred.uid = xatoui(argv[1]);
	container_cred.gid = xatoui(argv[2]);
	executor_cred.uid = xatoui(argv[3]);
	executor_cred.gid = xatoui(argv[4]);
	cgroup_config.title = argv[5];
	cgroup_config.parent = argv[6];

	umask(0777);

	xlimit(RLIMIT_FSIZE, 0);
	xlimit(RLIMIT_MEMLOCK, 0);
	xlimit(RLIMIT_MSGQUEUE, 0);
	xlimit(RLIMIT_RTPRIO, 0);
	xlimit(RLIMIT_RTTIME, 0);
	xlimit(RLIMIT_SIGPENDING, 0); // applies only to sigqueue

	if (pipe2(syncpipe, O_CLOEXEC) != 0)
		xerror("pipe");

	pid_t child_pid = xclone(child_main, CLONE_NEWCGROUP | CLONE_NEWIPC | CLONE_NEWNET | CLONE_NEWNS | CLONE_NEWPID | CLONE_NEWUSER | CLONE_NEWUTS | SIGCHLD);

	return parent_main(child_pid);
}

static int parent_main(pid_t child_pid)
{
	xclose(GATE_NULL_FD);
	xclose(GATE_CONTROL_FD);
	xclose(syncpipe[0]);

	init_cgroup(child_pid, &cgroup_config);

	xclear_caps();

	// cgroup configured

	xwrite_uid_map(child_pid, container_cred.uid, executor_cred.uid);
	xwrite_gid_map(child_pid, container_cred.gid, executor_cred.gid);

	// user namespace configured

	xclose(syncpipe[1]); // wake child up

	return wait_for_child(child_pid);
}

static int child_main(void *dummy_arg)
{
	xset_pdeathsig(SIGKILL);

	if (prctl(PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0) != 0)
		xerror("PR_SET_NO_NEW_PRIVS");

	xclose(syncpipe[1]);
	xread_until_eof(syncpipe[0]); // wait for parent to wakes us up
	xclose(syncpipe[0]);

	// user namespace and cgroup have been configured by parent

	int executor_fd = xopen_executor_and_loader();

	if (setgroups(0, NULL) != 0)
		xerror("setgroups to empty list");

	uint64_t rand;
	xgetentropy(&rand, sizeof (rand));

	char *tmp_proc;
	if (asprintf(&tmp_proc, "/tmp/.%016lx", rand) < 0)
		xerror("asprintf");

	// "container" credentials
	if (setreuid(2, 2) != 0)
		xerror("setuid for container setup");
	if (setregid(2, 2) != 0)
		xerror("setgid for container setup");

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

	if (mkdir(tmp_proc, 0) != 0)
		xerror("mkdir inside small tmpfs");

	if (mount("proc", tmp_proc, "proc", mount_options, NULL) != 0)
		xerror("mount proc inside would-be root");

	if (mkdir("/tmp/x", 0) != 0)
		xerror("mkdir inside small tmpfs");

	xpivot_root("/tmp", "/tmp/x");

	const char *proc = tmp_proc + 4; // remove "/tmp" prefix

	if (chdir("/") != 0)
		xerror("chdir to new root");

	if (umount2("/x", MNT_DETACH) != 0)
		xerror("umount old root");

	// keep the x directory so that the filesystem remains full inode-wise

	mount_options |= MS_RDONLY;

	if (mount("", "/", "", MS_REMOUNT|mount_options, NULL) != 0)
		xerror("remount new root as read-only");

	if (chdir("/x") != 0)
		xerror("chdir to work dir");

	// "executor" credentials
	if (setreuid(3, 3) != 0)
		xerror("setuid for execution");
	if (setregid(3, 3) != 0)
		xerror("setgid for execution");

	xclear_caps();

	if (prctl(PR_CAP_AMBIENT, PR_CAP_AMBIENT_CLEAR_ALL, 0, 0, 0) != 0)
		xerror("PR_CAP_AMBIENT_CLEAR_ALL");

	char *executor;
	char *loader;

	if (asprintf(&executor, "%s/self/fd/%d", proc, executor_fd) < 0)
		xerror("asprintf");

	if (asprintf(&loader, "%s/self/fd/%d", proc, GATE_LOADER_FD) < 0)
		xerror("asprintf");

	// enable scheduler's autogroup feature
	if (setsid() < 0)
		xerror("setsid");

	long page = sysconf(_SC_PAGESIZE);
	if (page <= 0)
		xerror("sysconf _SC_PAGESIZE");

	xlimit(RLIMIT_AS, GATE_LIMIT_AS);
	xlimit(RLIMIT_CORE, 0);
	xlimit(RLIMIT_STACK, (GATE_LOADER_STACK_SIZE+page-1) & ~(page-1));

	xdup2(GATE_NULL_FD, STDERR_FILENO);
	close(GATE_NULL_FD);

	char *envp[] = {loader, NULL};
	char **empty = envp + 1;
	execve(executor, empty, envp);
	// stderr doesn't work anymore
	return 2;
}
