# Gate

Run untrusted code from anonymous sources.  Instead of sending messages
composed of passive data, send programs which can react to their environment.
Migrate or duplicate running applications across hosts and computer
architectures.
See [Introduction to Gate](https://savo.la/introduction-to-gate.html).

- License: [3-clause BSD](LICENSE)
- Author: Timo Savola <timo.savola@iki.fi>


## Foundations

[WebAssembly](https://webassembly.org) is the interchange format of the user
programs.  However, the APIs are different from the browsers' usual WebAssembly
environments.  See low-level [C API](C.md) or the higher-level
[Rust crate](https://crates.io/crates/gain) for details.

The sandboxing and containerization features of the Linux kernel provide layers
of security in addition to WebAssembly.  See [Security](Security.md) for
details.

Gate *services* are akin to syscalls, but they work differently.  New services
can be added easily, and available services are discovered at run time.  See
[Service implementation](Service.md) for details.


## Building blocks

Gate appears as [Go](https://golang.org) packages and programs.  The execution
mechanism is implemented in C and assembly.  It is highly Linux-dependent.
x86-64 and ARM64 are supported.

Important Go packages:

  - [**wag**](https://godoc.org/gate.computer/wag):
    The WebAssembly compiler (implemented in a
    [separate repository](https://gate.computer/wag)).

  - [**gate/runtime**](https://godoc.org/gate.computer/gate/runtime):
    Core functionality.  Interface to the execution mechanism.

  - [**gate/image**](https://godoc.org/gate.computer/gate/image):
    Low-level executable building and instance management.

  - [**gate/build**](https://godoc.org/gate.computer/gate/build):
    High-level executable building and snapshot restoration.

  - [**gate/server/web**](https://godoc.org/gate.computer/gate/server/web):
    HTTP server component which executes your code on purpose.  It has a
    [RESTful API](Web.md), but some actions can be invoked also via websocket.

  - [**gate/service**](https://godoc.org/gate.computer/gate/service):
    Service implementation support and built-in services.

See the complete [list of Go packages](https://godoc.org/gate.computer/gate).

Programs:

  - **gate**:
    Command-line client for local daemon and remote servers.  Uses SSH keys
    (Ed25519) for authentication.

  - **gate-daemon**:
    D-Bus daemon for running and managing instances and wasm modules locally.

  - **gate-server**:
    Standalone web server which can serve the public or require authentication.

  - **gate-runtime**:
    For optionally preconfiguring the execution environment for daemon/server,
    e.g. as a system service.

The available services are determined by what is built into the gate-daemon and
gate-server programs.  The versions provided by this Go module include only the
services implemented in this repository.  See [extension](Extension.md) about
bundling additional services.


## Objectives

While code is data, most of the time data cannot be treated as code for safety
reasons.  Change that at the Internet level.  Data encapsulated in code can
describe and transform itself.

Application portability.  Migrate processes between mobile devices and servers
when circumstances change: user presence, resource availability or demand,
continuity etc.

Overhead needs to be low enough so that the system can be practical.  Low
startup latency for request processing.  Low memory overhead for high density
of continually running programs.


## Work in progress

  - [x] Linux x86-64 host support
  - [x] Android host support
  - [x] Support for WebAssembly version 1
  - [x] Planned security measures have been implemented
  - [x] HTTP server for running programs
  - [x] Client can communicate with the program it runs on the server
  - [x] Speculative execution security issue mitigations
  - [x] Pluggable authentication
  - [x] Load programs from IPFS
  - [x] Reconnect to program instance
  - [x] Snapshot
  - [x] Restore
  - [x] Mechanism for implementing external services in language agnostic way (gRPC)
  - [ ] Programs can discover and communicate with their peers on a server
  - [ ] Full ARM64 host support
  - [ ] [milestone/1](https://github.com/gate-computer/gate/milestone/1)
  - [ ] Clone programs locally or remotely (with or without snapshotting)
  - [ ] [milestone/2](https://github.com/gate-computer/gate/milestone/2)
  - [ ] Useful resource control policies need more thought (cgroup configuration etc.)
  - [ ] Stable APIs
  - [ ] Additional security measures (such as a SELinux profile)
  - [ ] Non-Linux host support

User program support:

  - [x] Low-level C API
  - [x] [Rust](https://crates.io/crates/gain) support
  - [ ] Go support
  - [ ] Approach for splitting WebAssembly app between browser (UI) and server (state)


## Requirements and build instructions

Run-time dependencies:

- Programs other than **gate** require Linux 5.2.  **gate**'s remote access
  features should work on any operating system, but are routinely tested only
  on Linux.

- D-Bus is used for communication between **gate** and **gate-daemon**,
  requiring D-Bus user service (dbus-user-session).  **gate** doesn't require
  D-Bus when accessing a remote server.

- Programs other than **gate** may need external tools depending on their
  configuration and [capabilities](Capabilities.md).

There are two approaches to building Gate: using Go directly, or via Make.


### Build using Go

Build requirements:

  - The **gate** client program requires Go 1.13
  - Other programs require Go 1.16

The Gate programs and programs importing Gate modules can be built normally
using `go install`, `go get` etc.  See the
[services](https://github.com/gate-computer/services#readme) repository for a
list of programs.

Gate runtime needs to execute some separately built binaries.  To make the
built Go programs self-contained, pre-built binaries are bundled into them by
default.  The pre-built binary files are under version control, and can be
rebuilt using `go generate`.  To disable bundling of pre-built binaries,
specify `-tags=gateexecdir` for the Go build command, and use Make to build and
install them separately.


### Build using Make

Build requirements:

  - Linux
  - C compiler
  - Go compiler
  - protobuf-compiler
  - libprotobuf-dev

The components implemented with C and assembly can be built using `make`.
`make bin` builds the Go programs without bundling the non-Go components in
them.

Additional requirements for `make check`:

  - Python 3
  - uidmap (shadow-utils)

See Makefile for more targets, such as `make install` and its variants.


## See also

- [Gain crate for Rust user programs](https://crates.io/crates/gain)
- [C API for user programs](C.md)
- [ABI for user programs](ABI.md)
- [Web server API](Web.md)
- [Service implementation](Service.md)
- [Build-time extensions](Extension.md)
- [Security](Security.md)
- [Container capabilities](Capabilities.md)
- [Go packages](https://godoc.org/gate.computer/gate)
- [wag](https://gate.computer/wag)

