# Security

Gate is designed to execute untrusted code from anonymous sources.  In order to
be useful, the untrusted programs must be able to access some interesting
services, or interact with other programs, all running on the same host.  This
document discusses the challenges which have been identified, and the measures
which have been implemented.

Threat categories in order of importance:

  1. Code execution or information disclosure in the context of the host
     environment, e.g. the operating system, possibly a hypervisor, and by
     extension the local network.

  2. Code execution or information disclosure in the context of the application
     embedding the Gate runtime and services, such as gate-server.

  3. Code execution or information disclosure in the context of other programs
     within the Gate execution environment.

  4. Denial of service by circumventing resource usage controls.

An example of particularly sensitive information is cryptographic keys.  They
may be actively used by the host operating system or gate-server, which may
make them vulnerable to side-channel attacks.


## Exposure

There are a few areas of contact between Gate and untrusted input.  If the
application embedding Gate is an internet-facing server, it also needs to deal
with other common security details; those are not covered by this document.


### 1. WebAssembly compiler

[Wag](https://gate.computer/wag) takes a wasm32 binary and generates machine
code.  It is implemented in Go, which helps with some traditional bug classes.
It doesn't use [unsafe](https://golang.org/pkg/unsafe/) operations or
concurrency, so it's as type safe as Go can be, and there are no data races in
the compiler.  While one of its objectives is compilation speed, it has been
implemented from the start with Gate's security-oriented use case in mind.
Nevertheless, it has a large attack surface.

The security aspects of the generated code are explained in the
[WebAssembly sandbox](#1-webassembly-sandbox) section.


### 2. Execution environment

The generated machine code is executed directly on the CPU.  See the [Execution
architecture](#execution-architecture) for details.


### 3. Services

The programs interact with Gate's service I/O loop and the available services,
implemented in Go.  (A service may be implemented via IPC/RPC, so other
programming languages may be involved aswell.)  Each service has its own
interface, so they must take care of their own input validation; as with
syscalls, complexity leads to bugs, and information is power.  A naive service
could enable side-channel attacks by providing too detailed information
(e.g. precise time).


## Execution architecture

The execution environment features several security mechanisms.  They are
listed starting with the innermost layer, which is in contact with the
untrusted code.


### 1. WebAssembly sandbox

[WebAssembly](http://webassembly.org) constrains programs to a logical sandbox,
and is designed for easy validation.  Particularly helpful details about wasm
programs are that they never store buffers on the call stack, and function
pointer target addresses are whitelisted by signature.
[Wag](https://gate.computer/wag) and Gate employ some additional safety
measures:

  - Programs are limited to 32-bit memory addressing, while the compiler
    targets 64-bit hosts.  Linear memory (heap) is mapped at a location which
    has at least 8 GB of unmapped address space around it, which puts other
    mappings beyond the maximum displacement which can be encoded by a wasm32
    program.  This is an additional layer of security on top of the
    WebAssembly-mandated memory bounds checking.  It mitigates
	[Spectre attack](https://spectreattack.com) variant 1.

  - Program code is mapped the same way as linear memory.

  - [Retpoline](https://support.google.com/faqs/answer/7625886) is used for
    indirect calls and jumps (x86-64).

  - The designated function return value register is cleared by void functions
    to avoid information leaks (e.g. internal pointers) if there would be a
    mismatch between the caller and the callee.


### 2. Seccomp sandbox

The process has [seccomp](https://en.wikipedia.org/wiki/Seccomp) enabled with a
very restrictive filter, so even if the WebAssembly sandbox could be breached,
arbitrary system calls cannot be made.

Possible operations:

  - Read from/write to the same file descriptors which the program could access
    via the unprivileged Gate runtime ABI functions.

  - Poll the file descriptors.

  - mprotect linear memory and stack regions as read/writable.  Of those, the
    only thing which is not already read/writable is the unallocated portion of
    linear memory, and the program could do that also via the unprivileged
    WebAssembly `memory.grow` function.  Runtime and program code protections
    cannot be changed.

  - Terminate the process with an arbitrary exit status.  It can be used to
    communicate a fake trap or error condition.

  - Call clock_gettime syscall with maximum resolution.

  - Call getcpu, gettimeofday and time syscalls via vDSO.


### 3. Process

The process is configured in various ways:

  - CORE, MEMLOCK, MSGQUEUE, NOFILE, NPROC, RTPRIO, RTTIME and SIGPENDING
    resource limits are set to zero.

  - AS, DATA, FSIZE and STACK resources are limited.

  - The process is not dumpable.

  - Unnecessary file descriptors are closed.

  - The initial stack is unmapped.  It prevents the comm text from being
    changed.

  - Initialization code is unmapped, retaining only the ABI functions needed at
    runtime.

  - Runtime code, and program code, data and stack are mapped at randomized
    addresses.

  - The process cannot gain privileges or capabilities via execve (e.g. if the
    executor or loader binary is misconfigured as setuid root).

  - Out-of-memory score adjustment is set to maximum.


### 4. Containment

The user program processes run in a container.  The container also includes an
init process which spawns and kills the programs.  The container has dedicated
cgroup, IPC, network, mount, pid, user and UTS namespaces.

By default, the user namespace contains only two unprivileged user ids (there
is no root user), one which owns the filesystem contents, and one which is used
to run the processes.  (If the `runtime.container.namespace.singleuid` config
option is set, there is only one unprivileged user id which is used for
everything, and it is mapped to the outside user who created the container.)

The mount namespace contains only a constrained read-only tmpfs as its root.

The UTS namespace has empty host and domain names.

(Setting the `runtime.container.namespace.disabled` config option invalidates
everything said in this section.)

