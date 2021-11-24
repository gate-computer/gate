// Code generated by internal/cmd/runtime-errors.  You can edit it a little bit.

package runtime

const (
	ERR_CONT_EXEC_EXECUTOR                 = 10
	ERR_EXEC_BRK                           = 11
	ERR_EXEC_PAGESIZE                      = 12
	ERR_EXEC_PDEATHSIG                     = 13
	ERR_EXEC_PERSONALITY_ADDR_NO_RANDOMIZE = 14
	ERR_EXEC_CLEAR_CAPS                    = 15
	ERR_EXEC_CLONE                         = 16
	ERR_EXEC_CLOSE                         = 17
	ERR_EXEC_SIGMASK                       = 18
	ERR_EXEC_SYSCONF_CLK_TCK               = 19
	ERR_EXEC_NO_NEW_PRIVS                  = 20
	ERR_EXEC_FCNTL_CLOEXEC                 = 21
	ERR_EXEC_FCNTL_GETFD                   = 22
	ERR_EXEC_SETRLIMIT_DATA                = 23
	ERR_EXEC_SETRLIMIT_NOFILE              = 24
	ERR_EXEC_SETRLIMIT_STACK               = 25
	ERR_EXEC_EPOLL_CREATE                  = 26
	ERR_EXEC_EPOLL_WAIT                    = 27
	ERR_EXEC_EPOLL_ADD                     = 28
	ERR_EXEC_EPOLL_MOD                     = 29
	ERR_EXEC_RECVMMSG                      = 30
	ERR_EXEC_MSG_LEN                       = 31
	ERR_EXEC_MSG_CTRUNC                    = 32
	ERR_EXEC_OP                            = 33
	ERR_EXEC_ID_RANGE                      = 34
	ERR_EXEC_CMSG_OP_MISMATCH              = 35
	ERR_EXEC_CMSG_LEVEL                    = 36
	ERR_EXEC_CMSG_TYPE                     = 37
	ERR_EXEC_CMSG_LEN                      = 38
	ERR_EXEC_CMSG_NXTHDR                   = 39
	ERR_EXEC_CREATE_PROCESS_BAD_STATE      = 40
	ERR_EXEC_WAIT_PROCESS_BAD_STATE        = 41
	ERR_EXEC_SEND                          = 42
	ERR_EXEC_SEND_ALIGN                    = 43
	ERR_EXEC_POLL_OTHER_EVENTS             = 44
	ERR_EXEC_POLL_OTHER_ID                 = 45
	ERR_EXEC_KILL                          = 46
	ERR_EXEC_WAITPID                       = 47
	ERR_EXEC_PRCTL_NOT_DUMPABLE            = 48
	ERR_EXEC_PRLIMIT_CPU                   = 49
	ERR_EXEC_PROCSTAT_OPEN                 = 50
	ERR_EXEC_PROCSTAT_READ                 = 51
	ERR_EXEC_PROCSTAT_PARSE                = 52
	ERR_EXEC_EPOLL_DEL                     = 53
	ERR_EXEC_FSTAT                         = 54
)

const (
	ERR_RT_READ                  = 4
	ERR_RT_WRITE                 = 5
	ERR_RT_DEBUG                 = 6
	ERR_RT_MPROTECT              = 7
	ERR_RT_MREMAP                = 8
	ERR_RT_CLOCK_GETTIME         = 9
	ERR_RT_POLL                  = 10
	ERR_SENTINEL_PDEATHSIG       = 11
	ERR_SENTINEL_CLOSE           = 12
	ERR_SENTINEL_SIGSUSPEND      = 13
	ERR_EXECHILD_DUP2            = 14
	ERR_EXECHILD_EXEC_LOADER     = 15
	ERR_LOAD_PDEATHSIG           = 18
	ERR_LOAD_SETRLIMIT_NOFILE    = 19
	ERR_LOAD_SETRLIMIT_NPROC     = 20
	ERR_LOAD_PRCTL_NOT_DUMPABLE  = 21
	ERR_LOAD_PERSONALITY_DEFAULT = 22
	ERR_LOAD_READ_INFO           = 23
	ERR_LOAD_MAGIC_1             = 24
	ERR_LOAD_MAGIC_2             = 25
	ERR_LOAD_MMAP_VECTOR         = 26
	ERR_LOAD_MPROTECT_VECTOR     = 27
	ERR_LOAD_MMAP_TEXT           = 28
	ERR_LOAD_MMAP_STACK          = 29
	ERR_LOAD_MMAP_HEAP           = 30
	ERR_LOAD_CLOSE_STATE         = 31
	ERR_LOAD_MUNMAP_STACK        = 32
	ERR_LOAD_SIGACTION           = 33
	ERR_LOAD_MUNMAP_LOADER       = 34
	ERR_LOAD_SECCOMP             = 35
	ERR_LOAD_ARG_ENV             = 36
	ERR_LOAD_NO_VDSO             = 37
	ERR_LOAD_FCNTL_INPUT         = 39
	ERR_LOAD_FCNTL_OUTPUT        = 40
	ERR_LOAD_MPROTECT_HEAP       = 41
	ERR_LOAD_CLOSE_TEXT          = 42
	ERR_LOAD_SETPRIORITY         = 43
	ERR_LOAD_NO_CLOCK_GETTIME    = 44
	ERR_LOAD_CLOCK_GETTIME       = 45
	ERR_LOAD_READ_TEXT           = 46
	ERR_LOAD_MREMAP_HEAP         = 47
)

var ExecutorErrors = [55]Error{
	10: {"ERR_CONT_EXEC_EXECUTOR", "runtime container", "failed to execute executor"},
	11: {"ERR_EXEC_BRK", "runtime executor", "brk call failed"},
	12: {"ERR_EXEC_PAGESIZE", "runtime executor", "sysconf PAGESIZE call failed"},
	13: {"ERR_EXEC_PDEATHSIG", "runtime executor", "failed to set process death signal"},
	14: {"ERR_EXEC_PERSONALITY_ADDR_NO_RANDOMIZE", "runtime executor", "failed to change personality to ADDR_NO_RANDOMIZE"},
	15: {"ERR_EXEC_CLEAR_CAPS", "runtime executor", "failed to clear capabilities"},
	16: {"ERR_EXEC_CLONE", "runtime executor", "clone call failed"},
	17: {"ERR_EXEC_CLOSE", "runtime executor", "file descriptor close error"},
	18: {"ERR_EXEC_SIGMASK", "runtime executor", "pthread_sigmask: failed to set mask"},
	19: {"ERR_EXEC_SYSCONF_CLK_TCK", "runtime executor", "sysconf CLK_TCK call failed"},
	20: {"ERR_EXEC_NO_NEW_PRIVS", "runtime executor", "prctl: failed to PR_SET_NO_NEW_PRIVS"},
	21: {"ERR_EXEC_FCNTL_CLOEXEC", "runtime executor", "fcntl: failed to add close-on-exec flag"},
	22: {"ERR_EXEC_FCNTL_GETFD", "runtime executor", "fcntl: failed to get file descriptor flags"},
	23: {"ERR_EXEC_SETRLIMIT_DATA", "runtime executor", "setrlimit: failed to set DATA limit"},
	24: {"ERR_EXEC_SETRLIMIT_NOFILE", "runtime executor", "setrlimit: failed to set NOFILE limit"},
	25: {"ERR_EXEC_SETRLIMIT_STACK", "runtime executor", "setrlimit: failed to set STACK limit"},
	26: {"ERR_EXEC_EPOLL_CREATE", "runtime executor", "epoll_create call failed"},
	27: {"ERR_EXEC_EPOLL_WAIT", "runtime executor", "epoll_wait call failed"},
	28: {"ERR_EXEC_EPOLL_ADD", "runtime executor", "epoll_ctl ADD call failed"},
	29: {"ERR_EXEC_EPOLL_MOD", "runtime executor", "epoll_ctl MOD call failed"},
	30: {"ERR_EXEC_RECVMMSG", "runtime executor", "recvmmsg call failed"},
	31: {"ERR_EXEC_MSG_LEN", "runtime executor", "received control message with unexpected length"},
	32: {"ERR_EXEC_MSG_CTRUNC", "runtime executor", "received truncated control message"},
	33: {"ERR_EXEC_OP", "runtime executor", "received message with unknown op type"},
	34: {"ERR_EXEC_ID_RANGE", "runtime executor", "process index out of bounds"},
	35: {"ERR_EXEC_CMSG_OP_MISMATCH", "runtime executor", "op type and control message expectation mismatch"},
	36: {"ERR_EXEC_CMSG_LEVEL", "runtime executor", "unexpected control message: not at socket level"},
	37: {"ERR_EXEC_CMSG_TYPE", "runtime executor", "unexpected control message type: no file descriptors"},
	38: {"ERR_EXEC_CMSG_LEN", "runtime executor", "unexpected control message length"},
	39: {"ERR_EXEC_CMSG_NXTHDR", "runtime executor", "multiple control message headers per recvmsg"},
	40: {"ERR_EXEC_CREATE_PROCESS_BAD_STATE", "runtime executor", "process index already in use"},
	41: {"ERR_EXEC_WAIT_PROCESS_BAD_STATE", "runtime executor", "event from pidfd with nonexistent process state"},
	42: {"ERR_EXEC_SEND", "runtime executor", "send call failed"},
	43: {"ERR_EXEC_SEND_ALIGN", "runtime executor", "sent unexpected number of bytes"},
	44: {"ERR_EXEC_POLL_OTHER_EVENTS", "runtime executor", "unexpected poll events"},
	45: {"ERR_EXEC_POLL_OTHER_ID", "runtime executor", "unknown poll event data"},
	46: {"ERR_EXEC_KILL", "runtime executor", "kill call failed"},
	47: {"ERR_EXEC_WAITPID", "runtime reaper", "waitpid call failed"},
	48: {"ERR_EXEC_PRCTL_NOT_DUMPABLE", "runtime executor", "prctl: failed to set not dumpable"},
	49: {"ERR_EXEC_PRLIMIT_CPU", "runtime executor", "prlimit CPU call failed"},
	50: {"ERR_EXEC_PROCSTAT_OPEN", "runtime executor", "failed to open /proc/<pid>/stat"},
	51: {"ERR_EXEC_PROCSTAT_READ", "runtime executor", "failed to read /proc/<pid>/stat"},
	52: {"ERR_EXEC_PROCSTAT_PARSE", "runtime executor", "/proc/<pid>/stat parse error"},
	53: {"ERR_EXEC_EPOLL_DEL", "runtime executor", "epoll_ctl DEL call failed"},
	54: {"ERR_EXEC_FSTAT", "runtime executor", "fstat call failed"},
}

var ProcessErrors = [48]Error{
	4:  {"ERR_RT_READ", "process runtime", "read call failed"},
	5:  {"ERR_RT_WRITE", "process runtime", "write call failed"},
	6:  {"ERR_RT_DEBUG", "process runtime", "debug write call failed"},
	7:  {"ERR_RT_MPROTECT", "process runtime", "mprotect call failed"},
	8:  {"ERR_RT_MREMAP", "process runtime", "mremap call failed"},
	9:  {"ERR_RT_CLOCK_GETTIME", "process runtime", "clock_gettime call failed"},
	10: {"ERR_RT_POLL", "process runtime", "poll call failed"},
	11: {"ERR_SENTINEL_PDEATHSIG", "sentinel process", "failed to set process death signal"},
	12: {"ERR_SENTINEL_CLOSE", "sentinel process", "TODO: ERR_SENTINEL_CLOSE"},
	13: {"ERR_SENTINEL_SIGSUSPEND", "sentinel process", "TODO: ERR_SENTINEL_SIGSUSPEND"},
	14: {"ERR_EXECHILD_DUP2", "process executor", "child: dup2 call failed"},
	15: {"ERR_EXECHILD_EXEC_LOADER", "process executor", "child: failed to execute loader"},
	18: {"ERR_LOAD_PDEATHSIG", "process loader", "failed to set process death signal"},
	19: {"ERR_LOAD_SETRLIMIT_NOFILE", "process loader", "child: setrlimit: failed to set NOFILE limit"},
	20: {"ERR_LOAD_SETRLIMIT_NPROC", "process loader", "child: setrlimit: failed to set NPROC limit"},
	21: {"ERR_LOAD_PRCTL_NOT_DUMPABLE", "process loader", "prctl: failed to set not dumpable"},
	22: {"ERR_LOAD_PERSONALITY_DEFAULT", "process loader", "failed to set default personality"},
	23: {"ERR_LOAD_READ_INFO", "process loader", "failed to read image info from input fd"},
	24: {"ERR_LOAD_MAGIC_1", "process loader", "magic number #1 mismatch"},
	25: {"ERR_LOAD_MAGIC_2", "process loader", "magic number #2 mismatch"},
	26: {"ERR_LOAD_MMAP_VECTOR", "process loader", "failed to allocate import vector via mmap"},
	27: {"ERR_LOAD_MPROTECT_VECTOR", "process loader", "mprotect: import vector write-protection failed"},
	28: {"ERR_LOAD_MMAP_TEXT", "process loader", "failed to mmap text section of image"},
	29: {"ERR_LOAD_MMAP_STACK", "process loader", "failed to mmap stack section of image"},
	30: {"ERR_LOAD_MMAP_HEAP", "process loader", "failed to mmap globals/memory section of image"},
	31: {"ERR_LOAD_CLOSE_STATE", "process loader", "failed to close program state fd"},
	32: {"ERR_LOAD_MUNMAP_STACK", "process loader", "failed to munmap initial stack"},
	33: {"ERR_LOAD_SIGACTION", "process loader", "sigaction call failed"},
	34: {"ERR_LOAD_MUNMAP_LOADER", "process loader", "failed to munmap loader .text and .rodata"},
	35: {"ERR_LOAD_SECCOMP", "process loader", "seccomp call failed"},
	36: {"ERR_LOAD_ARG_ENV", "process loader", "loader executed with wrong number of arguments or environment variables"},
	37: {"ERR_LOAD_NO_VDSO", "process loader", "vdso address not found in auxiliary vector"},
	39: {"ERR_LOAD_FCNTL_INPUT", "process loader", "failed to set input file flags"},
	40: {"ERR_LOAD_FCNTL_OUTPUT", "process loader", "failed to set output file flags"},
	41: {"ERR_LOAD_MPROTECT_HEAP", "process loader", "mprotect: globals/memory protection failed"},
	42: {"ERR_LOAD_CLOSE_TEXT", "process loader", "failed to close program text fd"},
	43: {"ERR_LOAD_SETPRIORITY", "process loader", "TODO: ERR_LOAD_SETPRIORITY"},
	44: {"ERR_LOAD_NO_CLOCK_GETTIME", "process loader", "clock_gettime not found in vDSO ELF"},
	45: {"ERR_LOAD_CLOCK_GETTIME", "process loader", "clock_gettime call failed"},
	46: {"ERR_LOAD_READ_TEXT", "process loader", "failed to read text section of image"},
	47: {"ERR_LOAD_MREMAP_HEAP", "process loader", "failed to mremap globals/memory section of image"},
}

var ErrorsInitialized struct{}
