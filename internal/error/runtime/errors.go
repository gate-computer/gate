// Code generated by internal/cmd/runtime-errors.  You can edit it a little bit.

package runtime

var ExecutorErrors = [72]Error{
	10: {"ERR_CONT_EXEC_EXECUTOR", "runtime container", "failed to execute executor"},
	11: {"ERR_EXEC_PRCTL_NOT_DUMPABLE", "runtime executor", "prctl: failed to set not dumpable"},
	12: {"ERR_EXEC_SETRLIMIT_DATA", "runtime executor", "setrlimit: failed to set DATA limit"},
	13: {"ERR_EXEC_FCNTL_GETFD", "runtime executor", "fcntl: failed to get file descriptor flags"},
	14: {"ERR_EXEC_FCNTL_CLOEXEC", "runtime executor", "fcntl: failed to add close-on-exec flag"},
	16: {"ERR_EXEC_SIGMASK", "runtime executor", "pthread_sigmask: failed to set mask"},
	17: {"ERR_EXEC_KILL", "runtime executor", "kill call failed"},
	18: {"ERR_REAP_WAITPID", "runtime reaper", "waitpid call failed"},
	19: {"ERR_EXEC_PPOLL", "runtime executor", "ppoll call failed"},
	20: {"ERR_EXEC_RECVMSG", "runtime executor", "recvmsg call failed"},
	21: {"ERR_EXEC_SEND", "runtime executor", "send call failed"},
	22: {"ERR_EXEC_VFORK", "runtime executor", "vfork call failed"},
	23: {"ERR_EXEC_MSG_CTRUNC", "runtime executor", "received truncated control message"},
	24: {"ERR_EXEC_CMSG_LEVEL", "runtime executor", "unexpected control message: not at socket level"},
	25: {"ERR_EXEC_CMSG_TYPE", "runtime executor", "unexpected control message type: no file descriptors"},
	26: {"ERR_EXEC_CMSG_LEN", "runtime executor", "unexpected control message length"},
	27: {"ERR_EXEC_CMSG_NXTHDR", "runtime executor", "multiple control message headers per recvmsg"},
	28: {"ERR_EXEC_SENDBUF_OVERFLOW_CMSG", "runtime executor", "send buffer overflow on recvmsg"},
	29: {"ERR_EXEC_SENDBUF_OVERFLOW_REAP", "runtime executor", "send buffer overflow on waitpid"},
	30: {"ERR_EXEC_KILLBUF_OVERFLOW", "runtime executor", "kill buffer overflow"},
	31: {"ERR_EXEC_DEADBUF_OVERFLOW", "runtime executor", "dead pid buffer overflow"},
	32: {"ERR_EXEC_KILLMSG_PID", "runtime executor", "received kill message with invalid pid"},
	33: {"ERR_EXEC_PERSONALITY_ADDR_NO_RANDOMIZE", "runtime executor", "failed to change personality to ADDR_NO_RANDOMIZE"},
	34: {"ERR_EXEC_PRLIMIT", "runtime executor", "prlimit call failed"},
	36: {"ERR_EXEC_SETRLIMIT_STACK", "runtime executor", "setrlimit: failed to set STACK limit"},
	37: {"ERR_EXEC_PAGESIZE", "runtime executor", "TODO: ERR_EXEC_PAGESIZE"},
	43: {"ERR_REAP_SENTINEL", "runtime reaper", "sentinel process terminated unexpectedly"},
	44: {"ERR_EXEC_NODE_ALLOC", "runtime executor", "TODO: ERR_EXEC_NODE_ALLOC"},
	45: {"ERR_EXEC_BRK", "runtime executor", "TODO: ERR_EXEC_BRK"},
	46: {"ERR_EXEC_MAP_REMOVE", "runtime executor", "TODO: ERR_EXEC_MAP_REMOVE"},
	47: {"ERR_REAP_WRITEV", "runtime reaper", "TODO: ERR_REAP_WRITEV"},
	48: {"ERR_REAP_WRITE_ALIGN", "runtime reaper", "TODO: ERR_REAP_WRITE_ALIGN"},
	49: {"ERR_EXEC_MAP_PID", "runtime executor", "TODO: ERR_EXEC_MAP_PID"},
	50: {"ERR_EXEC_MAP_INSERT", "runtime executor", "TODO: ERR_EXEC_MAP_INSERT"},
	51: {"ERR_EXEC_OP", "runtime executor", "TODO: ERR_EXEC_OP"},
	52: {"ERR_EXEC_THREAD_ATTR", "runtime reaper", "TODO: ERR_EXEC_THREAD_ATTR"},
	53: {"ERR_EXEC_THREAD_CREATE", "runtime reaper", "TODO: ERR_EXEC_THREAD_CREATE"},
	54: {"ERR_EXEC_SIGACTION", "runtime reaper", "signal handler registration failed"},
	56: {"ERR_EXEC_PRLIMIT_CPU", "runtime executor", "TODO: ERR_EXEC_PRLIMIT_CPU"},
	57: {"ERR_EXEC_FORK_SENTINEL", "runtime executor", "sentinel process fork failed"},
	58: {"ERR_EXEC_KILL_SENTINEL", "runtime executor", "sentinel process kill failed"},
	60: {"ERR_EXEC_MSG_LEN", "runtime executor", "TODO: ERR_EXEC_MSG_LEN"},
	62: {"ERR_EXEC_CMSG_OP_MISMATCH", "runtime executor", "TODO: ERR_EXEC_CMSG_OP_MISMATCH"},
	63: {"ERR_EXEC_ID_RANGE", "runtime executor", "TODO: ERR_EXEC_ID_RANGE"},
	64: {"ERR_EXEC_RAISE", "runtime repaer", "TODO: ERR_EXEC_RAISE"},
	65: {"ERR_EXEC_NO_NEW_PRIVS", "runtime executor", "prctl: failed to PR_SET_NO_NEW_PRIVS"},
	66: {"ERR_EXEC_CLEAR_CAPS", "runtime executor", "failed to clear capabilities"},
	67: {"ERR_EXEC_PROCSTAT_OPEN", "runtime executor", "failed to open /proc/PID/stat"},
	68: {"ERR_EXEC_PROCSTAT_READ", "runtime executor", "failed to read /proc/PID/stat"},
	69: {"ERR_EXEC_PROCSTAT_PARSE", "runtime executor", "/proc/PID/stat parse error"},
	70: {"ERR_EXEC_CLOSE", "runtime executor", "file descriptor close error"},
	71: {"ERR_EXEC_SIGNAL", "runtime executor", "failed to configure signal"},
}

var ProcessErrors = [49]Error{
	4:  {"ERR_RT_RECVFROM", "process runtime", "recvfrom call failed"},
	5:  {"ERR_RT_WRITE", "process runtime", "write call failed"},
	6:  {"ERR_RT_DEBUG", "process runtime", "debug write call failed"},
	7:  {"ERR_RT_MPROTECT", "process runtime", "mprotect call failed"},
	8:  {"ERR_RT_MREMAP", "process runtime", "mremap call failed"},
	9:  {"ERR_RT_CLOCK_GETTIME", "process runtime", "clock_gettime call failed"},
	11: {"ERR_SENTINEL_PRCTL_PDEATHSIG", "sentinel process", "TODO: ERR_SENTINEL_PRCTL_PDEATHSIG"},
	12: {"ERR_SENTINEL_CLOSE", "sentinel process", "TODO: ERR_SENTINEL_CLOSE"},
	13: {"ERR_SENTINEL_SIGSUSPEND", "sentinel process", "TODO: ERR_SENTINEL_SIGSUSPEND"},
	14: {"ERR_EXECHILD_DUP2", "process executor", "child: dup2 call failed"},
	15: {"ERR_EXECHILD_EXEC_LOADER", "process executor", "child: failed to execute loader"},
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
	36: {"ERR_LOAD_ARG_ENV", "process loader", "loader executed with arguments or environment"},
	37: {"ERR_LOAD_NO_VDSO", "process loader", "vdso address not found in auxiliary vector"},
	39: {"ERR_LOAD_FCNTL_INPUT", "process loader", "failed to set input file flags"},
	40: {"ERR_LOAD_FCNTL_OUTPUT", "process loader", "failed to set output file flags"},
	41: {"ERR_LOAD_MPROTECT_HEAP", "process loader", "mprotect: globals/memory protection failed"},
	42: {"ERR_LOAD_CLOSE_TEXT", "process loader", "failed to close program text fd"},
	43: {"ERR_LOAD_SETPRIORITY", "process loader", "TODO: ERR_LOAD_SETPRIORITY"},
	46: {"ERR_LOAD_NO_CLOCK_GETTIME", "process loader", "clock_gettime not found in vDSO ELF"},
	47: {"ERR_LOAD_READ_TEXT", "process loader", "failed to read text section of image"},
	48: {"ERR_LOAD_MREMAP_HEAP", "process loader", "failed to mremap globals/memory section of image"},
}

var ErrorsInitialized struct{}
