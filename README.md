# Gate

Run untrusted code safely.  Replace rigid HTTP requests and such with custom
programs that are executed on the server.  Migrate or duplicate live
applications across hosts and CPU architectures.
Create a [universal server](https://joearms.github.io/2013/11/21/My-favorite-erlang-program.html).
Gate is a toolkit for doing things like that.

- License: [3-clause BSD](LICENSE)
- Author: Timo Savola <timo.savola@iki.fi>


## Foundations

[WebAssembly](http://webassembly.org), or wasm, is the executable format of the
user programs.  However, the available APIs are completely different from the
browsers' WebAssembly environments.
See [Programming interface](Programming.md) for details.

Linux's sandboxing and containerization features provide layers of security in
addition WebAssembly's constraints.

*Services* are akin to syscalls: they define the usefulness of the programs.
Gate provides a few fundamental services, but is intended to be extended.
See [Service implementation](Service.md) for details.


## Building blocks

Gate appears as [Go](https://golang.org) packages and programs.  The execution
mechanism is highly Linux-dependent, implemented in C and assembly.  It needs
to be built separately (see below).  Currently only x86-64 is supported, but
64-bit ARM support is a primary goal.

Important packages:

  - [**wag**](https://godoc.org/github.com/tsavola/wag):
    The WebAssembly compiler
    (implemented in a [separate project](https://github.com/tsavola/wag)).

  - [**run**](https://godoc.org/github.com/tsavola/gate/run):
    Core functionality. Interface to the execution mechanism.

  - [**webserver**](https://godoc.org/github.com/tsavola/gate/webserver):
    HTTP server which executes your code on purpose.

  - [**service**](https://godoc.org/github.com/tsavola/gate/service):
    Default service implementations.

Server programs:

  - **gate-server**:
    Standalone HTTP server with the default services.

  - **gate-containerd**:
    For (optionally) preconfiguring the execution environment.

Client programs:

  - **gate-runner**:
    Run your programs locally, with the default services.

  - **gate-talk**:
    Chat with your peers on a server.
    See the [client-side client](examples/gate-talk/talk.go)
    and the [server-side client](examples/gate-talk/payload/talk.c) code examples.

See the complete [list of Go packages](https://godoc.org/github.com/tsavola/gate).


## Functional objectives

- Make untrusted code as safe as untrusted data.  Instead of doing multiple API
  calls over the internet, move part of the client logic next to the API (like
  GraphQL).  Client logic may keep responding to peers while the client UI is
  disconnected.

- Application portability (trusted or untrusted).  Migrate applications between
  phones and servers.  Snapshot, suspend, and resume applications in a portable
  way.  Low barrier to job provisioning.

- Support mainstream servers, desktops and mobile devices as much as possible.
  Linux servers and Android devices are the apparent targets.  Windows and
  macOS desktops could run the user programs in a Linux virtual machine, while
  the UI is a thin client (e.g. in the browser).  Alternatively a program can
  be run in the browser's WebAssembly VM, if advanced features are not needed.
  The available services in each environment may differ: desktops and mobile
  devices have display and input capabilities which servers lack, but a server
  could continue running a large program which doesn't fit in a phone's memory.


## Non-functional objectives

- Security.

- Low enough overhead to be useful.  That means low startup latency and memory
  usage.


## Work in progress

Primary goals:

  - [x] Supports simple C/C++ programs (mostly limited by immature toolchain)
  - [x] Linux x86-64 host support
  - [x] Partial support for running unmodified programs in the browser
  - [x] All planned security measures have been implemented
  - [x] Bare-bones HTTP server for running programs
  - [x] Client can communicate with the program it runs on the server
  - [x] Programs can discover and communicate with their peers on a server
  - [ ] Support all WebAssembly instructions (wag is missing some floating-point ops)
  - [ ] 64-bit ARM host support, followed by Android support
  - [ ] Suspend, snapshot, restore (wag already has support)
  - [ ] Support resuming communication with program instance if connection dies
  - [ ] Expose program instance at some type of internet endpoint to implement ad-hoc servers
  - [ ] Clone programs locally or remotely (with or without snapshotting)
  - [ ] Service for integrating services in a programmer-friendly way (like a D-Bus bridge)
  - [ ] Useful resource control policies need more thought (cgroup configuration etc.)
  - [ ] Design pluggable authentication, and how it affects resource control policy
  - [ ] Stable APIs

Secondary goals:

  - [ ] Support for more complex, real-world programs (needs toolchain/ecosystem support)
  - [ ] Non-Linux host support


## Build requirements

Due to the nature of the Go toolchain, it's best to checkout the Git repository
at `$GOPATH/src/github.com/tsavola/gate`.  If you haven't set GOPATH, it
defaults to `~/go`.

The non-Go components can be built with `make`.  They require:

  - Linux
  - gcc or clang
  - pkg-config
  - libcap-dev
  - libsystemd-dev unless CGROUP_BACKEND=none is specified for make

After that, capabilities need to be granted by running `make capabilities` as
root (or in some other way; see [Installation security notes](run/container/Security.md)).
That requires:

  - libcap2-bin

The Go programs can be built with `make bin`.  It downloads some dependencies
to various directories under GOPATH.  In addition to them, these are required:

  - Go 1.8 (or 1.7 if you don't build the webserver)
  - libcapstone-dev is needed by gate-runner

The programming interface libraries can be built with `make devlibs`:

  - Git submodules need to be checked out
  - wag-toolchain is used via Docker by default, but a manually built one can
    also be used by setting TOOLCHAINDIR for make

See the Makefile for more interesting targets like `check` and `benchmark`.
The capabilities need to be granted for them to work.

(The dependencies are listed using Debian/Ubuntu package names.  Other Linux
distributions may use other names.)


## See also

- [Programming interface](Programming.md)
- [ABI](ABI.md)
- [Service implementation](Service.md)
- [Installation security notes](run/container/Security.md)
- [Go packages](https://godoc.org/github.com/tsavola/gate)
- [wag](https://github.com/tsavola/wag)
- [wag-toolchain](https://github.com/tsavola/wag-toolchain)

