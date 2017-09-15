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

[Wag](https://godoc.org/github.com/tsavola/wag) takes a wasm32 binary and
generates machine code.  It is implemented in Go, which helps with some
traditional bug classes.  While one of its objectives is compilation speed, it
has been implemented from the start with Gate's security-oriented use case in
mind.  Nevertheless, it has a large attack surface.

The security aspects of the generated code are explained in the
[WebAssembly sandbox](#1-webassembly-sandbox) section.


### 2. Execution environment

The generated machine code is executed directly on the CPU.  See the [Execution
architecture](#execution-architecture) for details.


### 3. Services

The programs interact with Gate's service I/O loop and the available services,
implemented in Go.  (A service may be implemented via IPC/RPC, so other
programming languages may be involved aswell.)  Each service has its own
interface, and must of its own input validation; as with syscalls, complexity
leads to bugs, and information is power.  A naive service could enable
side-channel attacks by providing too detailed information (e.g. precise time).


## Execution architecture

The execution environment features several security mechanisms.  They are
listed starting with the innermost layer, which is in contact with the
untrusted code.


### 1. WebAssembly sandbox

[WebAssembly](http://webassembly.org) constrains programs to a logical sandbox,
and it's designed for easy validation.  Particularly helpful details about wasm
programs are that they never store buffers in the call stack, and function
pointer targets addresses are whitelisted by signature.  The [Wag
compiler](https://github.com/tsavola/wag) has some additional design principles
for fool-proofing the generated code:

  - Programs are limited to 32-bit memory addressing, while the compiler
    targets 64-bit hosts.  Linear memory (heap) is mapped at a location which
    has at least 2GB of unmapped address space in each direction, and is
    accessed using instructions with at most 32-bit signed displacement; a bug
    in bounds checking (which affects only the displacement) cannot cause other
    memory mappings to be accessed.

  - Program code is mapped the same way as linear memory.

  - The designated function return value register is cleared by void functions
    to avoid information leaks (e.g. internal pointers) if there would be a
    mismatch between the caller and the callee.


### 2. Seccomp sandbox

The process has [seccomp](https://en.wikipedia.org/wiki/Seccomp) enabled in
strict mode: even if the WebAssembly sandbox could be breached, arbitrary
syscalls cannot be made.

Permitted operations:

  - Read from/write to the same file descriptors which the program could access
    via the unprivileged Gate runtime ABI functions.

  - Terminate the process with an arbitrary exit status.

  - Call clock_gettime, getcpu, gettimeofday and time syscalls via vDSO.  (This
    is worrisome, as it may enable timing attacks.)


### 3. Process

The process is configured in various ways:

  - CORE, DATA, FSIZE, MEMLOCK, MSGQUEUE, NPROC, RTPRIO, RTTIME and SIGPENDING
    resource limits are set to zero.

  - AS, NOFILE and STACK resource limit is set to small values.

  - The process is not dumpable.

  - The process is killed if the rdtsc instruction is executed.

  - Nice value is set to maximum (19).

  - Unnecessary file descriptors are closed.

  - The initial stack is unmapped.  It prevents the comm text from being set.

  - Initialization code is unmapped, retaining only the ABI functions needed at
    runtime.

  - Program code, data and stack are mapped at randomized addresses.
    (Read-only data is mapped at a fixed address.)


### 4. Containerization

The program processes run in a container.  It also includes an init process
which spawns and kills the programs.  The container has dedicated cgroup, IPC,
network, mount, pid, user and UTS namespaces.

The user namespace contains only two unprivileged user ids, one of which is
used to run the processes.

The mount namespace contains only a constrained tmpfs as root, and proc mounted
at a directory with a randomized name.

The UTS namespace has empty host and domain names.

Processes running in the container are configured in these ways:

  - A process is killed automatically if its parent dies.

  - The processes cannot be granted capabilities by any means.

  - Out-of-memory score adjustment is set to maximum.

