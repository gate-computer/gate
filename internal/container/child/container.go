// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package child

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"unsafe"

	"gate.computer/internal/container/common"
	runtimeerrors "gate.computer/internal/error/runtime"
	"golang.org/x/sys/unix"
)

// Additional file descriptors passed from container to executor.
const (
	procFD = 7
)

const (
	executorStackSize = 65536 // Depends on target architecture.
	loaderStackSize   = 12288 // 3 pages.
)

const limitAS = loaderStackSize +
	0x00001000 + // loader
	0x00001000 + // runtime
	0x80000000 + // text
	0x80000000 + // stack
	0x00001000 + // globals
	0x80000000 + // memory
	0

func ignoreSignals() {
	var ignore []os.Signal

	for sig := syscall.Signal(1); sig <= syscall.Signal(63); sig++ {
		switch sig {
		case unix.SIGBUS,
			unix.SIGCHLD,
			unix.SIGFPE,
			unix.SIGILL,
			unix.SIGKILL,
			unix.SIGSEGV,
			unix.SIGSTOP,
			unix.SIGTERM,
			unix.SIGXCPU:

		default:
			ignore = append(ignore, sig)
		}
	}

	signal.Ignore(ignore...)
}

func execveat(dirfd int, pathname string, argv, envv []string, flags int) error {
	pathnamep, err := syscall.BytePtrFromString(pathname)
	if err != nil {
		return err
	}

	argvp, err := syscall.SlicePtrFromStrings(argv)
	if err != nil {
		return err
	}

	envvp, err := syscall.SlicePtrFromStrings(envv)
	if err != nil {
		return err
	}

	_, _, errno := syscall.RawSyscall6(
		unix.SYS_EXECVEAT,
		uintptr(dirfd),
		uintptr(unsafe.Pointer(pathnamep)),
		uintptr(unsafe.Pointer(&argvp[0])),
		uintptr(unsafe.Pointer(&envvp[0])),
		uintptr(flags),
		0,
	)
	if errno != 0 {
		return fmt.Errorf("execveat: %w", errno)
	}

	return nil
}

func setrlimit(name string, resource int, value uint64) error {
	rlim := &syscall.Rlimit{
		Cur: value,
		Max: value,
	}

	if err := syscall.Setrlimit(resource, rlim); err != nil {
		return fmt.Errorf("setting %s resource limit to %d: %w", name, value, err)
	}

	return nil
}

func openProcPath(path string) error {
	fd, err := syscall.Open(path, unix.O_PATH|unix.O_DIRECTORY, 0)
	if err != nil {
		return fmt.Errorf("opening %s: %w", path, err)
	}
	if fd != procFD {
		return fmt.Errorf("unexpected file descriptor allocated for procfs: %d", fd)
	}
	return nil
}

func setCred(id int) error {
	// syscall package's implementations use AllThreadsSyscall.
	if err := syscall.Setreuid(id, id); err != nil {
		return fmt.Errorf("setting real and effective user ids to %d inside container: %w", id, err)
	}
	if err := syscall.Setregid(id, id); err != nil {
		return fmt.Errorf("setting real and effective group ids to %d inside container: %w", id, err)
	}
	return nil
}

func furnishNamespaces() error {
	// UTS namespace

	if err := syscall.Sethostname(nil); err != nil {
		return fmt.Errorf("setting hostname to empty string: %w", err)
	}

	if err := syscall.Setdomainname(nil); err != nil {
		return fmt.Errorf("setting domain name to empty string: %w", err)
	}

	// Mount namespace

	if err := syscall.Mount("", "/", "", unix.MS_PRIVATE|unix.MS_REC, ""); err != nil {
		return fmt.Errorf("remounting old root as private recursively: %w", err)
	}

	var mountOptions uintptr = unix.MS_NODEV | unix.MS_NOEXEC | unix.MS_NOSUID

	// Abuse /tmp as staging area for new root.
	if err := syscall.Mount("tmpfs", "/tmp", "tmpfs", mountOptions, "mode=0,nr_blocks=1,nr_inodes=2"); err != nil {
		return fmt.Errorf("mounting small tmpfs at /tmp: %w", err)
	}

	if err := os.Mkdir("/tmp/proc", 0); err != nil {
		return err
	}

	// For some reason this causes EPERM if done after pivot_root...
	if err := syscall.Mount("proc", "/tmp/proc", "proc", mountOptions, "hidepid=2"); err != nil {
		return fmt.Errorf("mounting /tmp/proc: %w", err)
	}

	if err := openProcPath("/tmp/proc"); err != nil {
		return err
	}

	if err := syscall.Unmount("/tmp/proc", unix.MNT_DETACH); err != nil {
		return fmt.Errorf("unmounting /tmp/proc: %w", err)
	}

	if err := os.Remove("/tmp/proc"); err != nil {
		return err
	}

	if err := os.Mkdir("/tmp/x", 0); err != nil {
		return err
	}

	if err := syscall.PivotRoot("/tmp", "/tmp/x"); err != nil {
		return fmt.Errorf("pivoting root: %w", err)
	}

	if err := os.Chdir("/"); err != nil {
		return err
	}

	if err := syscall.Unmount("/x", unix.MNT_DETACH); err != nil {
		return fmt.Errorf("unmounting old root: %w", err)
	}

	// Sit in the directory so that it remains busy and keeps the filesystem
	// full inode-wise.

	if err := os.Chdir("/x"); err != nil {
		return err
	}

	// Read-only filesystem

	mountOptions |= unix.MS_RDONLY

	if err := syscall.Mount("", "/", "", unix.MS_REMOUNT|mountOptions, ""); err != nil {
		return fmt.Errorf("remounting new root as read-only: %w", err)
	}

	return nil
}

func Exec() {
	fmt.Fprintln(os.Stderr, childMain())
	os.Exit(1)
}

func childMain() error {
	var (
		namespaceDisabled bool
		singleUID         bool
	)

	for _, arg := range os.Args[1:] {
		switch arg {
		case common.ArgNamespaceDisabled:
			namespaceDisabled = true

		case common.ArgSingleUID:
			singleUID = true
		}
	}

	ignoreSignals()

	if err := setupBinaries(); err != nil {
		return err
	}

	if _, err := io.Copy(io.Discard, os.Stdin); err != nil {
		return err
	}

	// At this point user namespace and cgroup have been configured by parent process.

	if err := syscall.Dup3(unix.Stdout, unix.Stdin, 0); err != nil { // stdin = /dev/null
		return fmt.Errorf("duplicating stdout as stdin: %w", err)
	}

	// Executor will readjust open file limit appropriately.  Handle error then.
	setrlimit("NOFILE", unix.RLIMIT_NOFILE, 1048576)

	if common.Sandbox {
		syscall.Umask(0o777)

		if err := setrlimit("FSIZE", unix.RLIMIT_FSIZE, limitFSIZE); err != nil {
			return err
		}
		if err := setrlimit("MEMLOCK", unix.RLIMIT_MEMLOCK, 0); err != nil {
			return err
		}
		if err := setrlimit("MSGQUEUE", unix.RLIMIT_MSGQUEUE, 0); err != nil {
			return err
		}
		if err := setrlimit("RTPRIO", unix.RLIMIT_RTPRIO, 0); err != nil {
			return err
		}
		if err := setrlimit("SIGPENDING", unix.RLIMIT_SIGPENDING, 0); err != nil { // Applies to sigqueue.
			return err
		}
	}

	if common.Sandbox && !namespaceDisabled {
		if err := setCred(common.ContainerCred); err != nil {
			return err
		}

		if !singleUID {
			// syscall package's implementation uses AllThreadsSyscall.
			if err := syscall.Setgroups(nil); err != nil {
				return fmt.Errorf("setgroups: %w", err)
			}
		}

		if err := furnishNamespaces(); err != nil {
			return err
		}

		if !singleUID {
			if err := setCred(common.ExecutorCred); err != nil {
				return err
			}
		}

		if err := setrlimit("AS", unix.RLIMIT_AS, limitAS); err != nil {
			return err
		}
		if err := setrlimit("CORE", unix.RLIMIT_CORE, 0); err != nil {
			return err
		}
		if err := setrlimit("STACK", unix.RLIMIT_STACK, executorStackSize); err != nil {
			return err
		}
	} else {
		fmt.Fprintln(os.Stderr, "container: disabled - sharing namespaces with host!")

		if err := openProcPath("/proc"); err != nil {
			return err
		}
	}

	runtime.LockOSThread() // Keep thread locked from capset to exec.

	if err := threadCapsetZero(); err != nil {
		return err
	}

	if err := unix.Prctl(unix.PR_CAP_AMBIENT, unix.PR_CAP_AMBIENT_CLEAR_ALL, 0, 0, 0); err != nil {
		return fmt.Errorf("clearing ambient capabilities (prctl): %w", err)
	}

	// This needs to be done after final credential change.
	if err := unix.Prctl(unix.PR_SET_PDEATHSIG, uintptr(unix.SIGKILL), 0, 0, 0); err != nil {
		return fmt.Errorf("setting process death signal: %w", err)
	}
	if os.Getppid() == 1 {
		panic(syscall.Kill(os.Getpid(), unix.SIGKILL))
	}

	if !runtimeDebug {
		if err := syscall.Dup3(unix.Stdout, unix.Stderr, 0); err != nil { // stderr = /dev/null
			return fmt.Errorf("duplicating stdout as stderr: %w", err)
		}
	}

	args := append([]string{common.ExecutorFilename}, os.Args[1:]...)
	err := execveat(common.ExecutorFD, "", args, nil, unix.AT_EMPTY_PATH)
	if runtimeDebug {
		return fmt.Errorf("execveat: %w", err)
	}
	os.Exit(runtimeerrors.ERR_CONT_EXEC_EXECUTOR) // stderr doesn't work anymore.
	panic("unreachable")
}
