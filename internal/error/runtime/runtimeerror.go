// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"fmt"
)

type Error struct {
	Define string
	Subsys string
	Text   string
}

func (e *Error) Error() string     { return e.Text }
func (e *Error) Subsystem() string { return e.Subsys }

func ExecutorError(code int) error {
	if code < len(ExecutorErrors) {
		e := &ExecutorErrors[code]
		if e.Define != "" {
			return e
		}
	}

	return fmt.Errorf("unknown exit code %d", code)
}

func ProcessError(code int) error {
	if code < len(ProcessErrors) {
		e := &ProcessErrors[code]
		if e.Define != "" {
			return e
		}
	}

	return fmt.Errorf("unknown runtime process exit code %d", code)
}

var ExecutorErrors = [...]Error{
	{}, // Success.
	{}, // Container failure with message written to stderr.

	{"ERR_CONT_EXEC_EXECUTOR", "runtime container", "failed to execute executor"},
	{"ERR_EXEC_BRK", "runtime executor", "brk call failed"},
	{"ERR_EXEC_PAGESIZE", "runtime executor", "sysconf PAGESIZE call failed"},
	{"ERR_EXEC_PDEATHSIG", "runtime executor", "failed to set process death signal"},
	{"ERR_EXEC_PERSONALITY_ADDR_NO_RANDOMIZE", "runtime executor", "failed to change personality to ADDR_NO_RANDOMIZE"},
	{"ERR_EXEC_CLEAR_CAPS", "runtime executor", "failed to clear capabilities"},
	{"ERR_EXEC_CLONE", "runtime executor", "clone call failed"},
	{"ERR_EXEC_CLOSE", "runtime executor", "file descriptor close error"},
	{"ERR_EXEC_SIGMASK", "runtime executor", "pthread_sigmask: failed to set mask"},
	{"ERR_EXEC_SYSCONF_CLK_TCK", "runtime executor", "sysconf CLK_TCK call failed"},
	{"ERR_EXEC_NO_NEW_PRIVS", "runtime executor", "prctl: failed to PR_SET_NO_NEW_PRIVS"},
	{"ERR_EXEC_FCNTL_CLOEXEC", "runtime executor", "fcntl: failed to add close-on-exec flag"},
	{"ERR_EXEC_FCNTL_GETFD", "runtime executor", "fcntl: failed to get file descriptor flags"},
	{"ERR_EXEC_SETRLIMIT_DATA", "runtime executor", "setrlimit: failed to set DATA limit"},
	{"ERR_EXEC_SETRLIMIT_NOFILE", "runtime executor", "setrlimit: failed to set NOFILE limit"},
	{"ERR_EXEC_SETRLIMIT_STACK", "runtime executor", "setrlimit: failed to set STACK limit"},
	{"ERR_EXEC_EPOLL_CREATE", "runtime executor", "epoll_create call failed"},
	{"ERR_EXEC_EPOLL_WAIT", "runtime executor", "epoll_wait call failed"},
	{"ERR_EXEC_EPOLL_ADD", "runtime executor", "epoll_ctl ADD call failed"},
	{"ERR_EXEC_EPOLL_MOD", "runtime executor", "epoll_ctl MOD call failed"},
	{"ERR_EXEC_EPOLL_DEL", "runtime executor", "epoll_ctl DEL call failed"},
	{"ERR_EXEC_RECVMMSG", "runtime executor", "recvmmsg call failed"},
	{"ERR_EXEC_MSG_LEN", "runtime executor", "received control message with unexpected length"},
	{"ERR_EXEC_MSG_CTRUNC", "runtime executor", "received truncated control message"},
	{"ERR_EXEC_OP", "runtime executor", "received message with unknown op type"},
	{"ERR_EXEC_ID_RANGE", "runtime executor", "process index out of bounds"},
	{"ERR_EXEC_CMSG_OP_MISMATCH", "runtime executor", "op type and control message expectation mismatch"},
	{"ERR_EXEC_CMSG_LEVEL", "runtime executor", "unexpected control message: not at socket level"},
	{"ERR_EXEC_CMSG_TYPE", "runtime executor", "unexpected control message type: no file descriptors"},
	{"ERR_EXEC_CMSG_LEN", "runtime executor", "unexpected control message length"},
	{"ERR_EXEC_CMSG_NXTHDR", "runtime executor", "multiple control message headers per recvmsg"},
	{"ERR_EXEC_CREATE_PROCESS_BAD_STATE", "runtime executor", "process index already in use"},
	{"ERR_EXEC_WAIT_PROCESS_BAD_STATE", "runtime executor", "event from pidfd with nonexistent process state"},
	{"ERR_EXEC_SEND", "runtime executor", "send call failed"},
	{"ERR_EXEC_SEND_ALIGN", "runtime executor", "sent unexpected number of bytes"},
	{"ERR_EXEC_POLL_OTHER_EVENTS", "runtime executor", "unexpected poll events"},
	{"ERR_EXEC_POLL_OTHER_ID", "runtime executor", "unknown poll event data"},
	{"ERR_EXEC_KILL", "runtime executor", "kill call failed"},
	{"ERR_EXEC_WAITPID", "runtime reaper", "waitpid call failed"},
	{"ERR_EXEC_PRCTL_NOT_DUMPABLE", "runtime executor", "prctl: failed to set not dumpable"},
	{"ERR_EXEC_PRLIMIT_CPU", "runtime executor", "prlimit CPU call failed"},
	{"ERR_EXEC_PROCSTAT_OPEN", "runtime executor", "failed to open /proc/<pid>/stat"},
	{"ERR_EXEC_PROCSTAT_READ", "runtime executor", "failed to read /proc/<pid>/stat"},
	{"ERR_EXEC_PROCSTAT_PARSE", "runtime executor", "/proc/<pid>/stat parse error"},
	{"ERR_EXEC_FSTAT", "runtime executor", "fstat call failed"},
}

var ProcessErrors = [...]Error{
	{}, // Halted success.
	{}, // Halted failure.
	{}, // Terminated success.
	{}, // Terminated failure.

	{"ERR_RT_READ", "process runtime", "read call failed"},
	{"ERR_RT_READ8", "process runtime", "failed to read 8 bytes at once"},
	{"ERR_RT_WRITE", "process runtime", "write call failed"},
	{"ERR_RT_WRITE8", "process runtime", "failed to write 8 bytes at once"},
	{"ERR_RT_DEBUG", "process runtime", "debug: write call failed"},
	{"ERR_RT_DEBUG8", "process runtime", "debug: failed to write 8 bytes at once"},
	{"ERR_RT_MPROTECT", "process runtime", "mprotect call failed"},
	{"ERR_RT_MREMAP", "process runtime", "mremap call failed"},
	{"ERR_RT_CLOCK_GETTIME", "process runtime", "clock_gettime call failed"},
	{"ERR_RT_PPOLL", "process runtime", "ppoll call failed"},
	{"ERR_EXECHILD_DUP2", "process executor", "child: dup2 call failed"},
	{"ERR_EXECHILD_EXEC_LOADER", "process executor", "child: failed to execute loader"},
	{"ERR_LOAD_PDEATHSIG", "process loader", "failed to set process death signal"},
	{"ERR_LOAD_SETRLIMIT_NOFILE", "process loader", "child: setrlimit: failed to set NOFILE limit"},
	{"ERR_LOAD_SETRLIMIT_NPROC", "process loader", "child: setrlimit: failed to set NPROC limit"},
	{"ERR_LOAD_PRCTL_NOT_DUMPABLE", "process loader", "prctl: failed to set not dumpable"},
	{"ERR_LOAD_PERSONALITY_DEFAULT", "process loader", "failed to set default personality"},
	{"ERR_LOAD_READ_INFO", "process loader", "failed to read image info from input fd"},
	{"ERR_LOAD_READ_TEXT", "process loader", "failed to read text section of image"},
	{"ERR_LOAD_MAGIC_1", "process loader", "magic number #1 mismatch"},
	{"ERR_LOAD_MAGIC_2", "process loader", "magic number #2 mismatch"},
	{"ERR_LOAD_MMAP_VECTOR", "process loader", "failed to allocate import vector via mmap"},
	{"ERR_LOAD_MMAP_TEXT", "process loader", "failed to mmap text section of image"},
	{"ERR_LOAD_MMAP_STACK", "process loader", "failed to mmap stack section of image"},
	{"ERR_LOAD_MMAP_HEAP", "process loader", "failed to mmap globals/memory section of image"},
	{"ERR_LOAD_MPROTECT_VECTOR", "process loader", "mprotect: import vector write-protection failed"},
	{"ERR_LOAD_MPROTECT_HEAP", "process loader", "mprotect: globals/memory protection failed"},
	{"ERR_LOAD_MREMAP_HEAP", "process loader", "failed to mremap globals/memory section of image"},
	{"ERR_LOAD_CLOSE_TEXT", "process loader", "failed to close program text fd"},
	{"ERR_LOAD_CLOSE_STATE", "process loader", "failed to close program state fd"},
	{"ERR_LOAD_MUNMAP_STACK", "process loader", "failed to munmap initial stack"},
	{"ERR_LOAD_MUNMAP_LOADER", "process loader", "failed to munmap loader .text and .rodata"},
	{"ERR_LOAD_SIGACTION", "process loader", "sigaction call failed"},
	{"ERR_LOAD_SECCOMP", "process loader", "seccomp call failed"},
	{"ERR_LOAD_ARG_ENV", "process loader", "loader executed with wrong number of arguments or environment variables"},
	{"ERR_LOAD_NO_VDSO", "process loader", "vdso address not found in auxiliary vector"},
	{"ERR_LOAD_FCNTL_INPUT", "process loader", "failed to set input file flags"},
	{"ERR_LOAD_FCNTL_OUTPUT", "process loader", "failed to set output file flags"},
	{"ERR_LOAD_NO_CLOCK_GETTIME", "process loader", "clock_gettime not found in vDSO ELF"},
	{"ERR_LOAD_CLOCK_GETTIME", "process loader", "clock_gettime call failed"},
}
