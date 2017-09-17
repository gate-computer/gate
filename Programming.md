# Programming interface

This document deals with the construction of user programs which target [Gate](README.md).

The interface between the execution environment and the programs consists of
the [WebAssembly](http://webassembly.org) module specification
which the [W3C](https://www.w3.org/community/webassembly) is working on, and
the runtime ABI specified by the Gate project.

The wasm32 binary format acts as a container for Gate-specific programs.
The programs must be self-sufficient: unlike in the browser, it's not possible
to write JavaScript glue between the browser's functionality and the
WebAssembly program.  In other words,
currently binaries built with [binaryen](https://github.com/WebAssembly/binaryen)
don't work.  Everything has to be built around the import functions provided by
Gate.  Luckily the gate-toolchain produces binaries which do work (see below).


## API

This is a description of Gate's C API.  See the [ABI](ABI.md) specification for
language-agnostic details.

The API declarations and implementations are in the [`gate.h`](include/gate.h)
header file.  It consists of a few functions which are primarily used to access
a packet-based I/O interface.


### Primitives

#### Runtime constants

```c
const int gate_abi_version;
```
> The ABI version.  The current version is 0.


```c
const int32_t gate_arg;
```
> The argument.


```c
const size_t gate_max_packet_size;
```
> The largest I/O packet size the runtime supports.  It's a fatal error to send
> a larger packet, or to call the `gate_recv_packet` function with a smaller
> buffer.


#### Functions

```c
void gate_debug(const char *s);
```
> Write a string to the runtime debug log, if enabled.  Defining `NDEBUG`
> during build also disables it.


```c
void gate_exit(int status);
```
> Terminate the program immediately.  Status value 0 indicates success and 1
> indicates failure.  Other values are reserved.


```c
size_t gate_recv_packet(void *buf, size_t size, unsigned flags);
```
> Receive a complete packet into a buffer.  *size* must be at least
> `gate_max_packet_size`.  If the `GATE_RECV_FLAG_NONBLOCK` flag is set, the
> call doesn't block, but the packet might not be received.  The size of the
> packet is returned, or 0 if one was not received.


```c
void gate_send_packet(const struct gate_packet *packet);
```
> Send a complete packet.  The size is specified via the packet header.
>
> The call may or may not block.  If a program relies on non-blocking send, it
> has to request to be notified when that is possible by setting the
> `GATE_PACKET_FLAG_POLLOUT` flag in the packet of a previous send.  (The
> program should include the flag in every packet if it wants to do only
> non-blocking sends.)
>
> If a packet is received with the `GATE_PACKET_FLAG_POLLOUT` flag set, it
> means that (at least) one packet can be sent without blocking.  Multiple
> requests may be combined: if notification was requested multiple times before
> the notification was delivered, the state is unknown.


#### Packet header

```c
struct gate_packet {
	uint32_t size;
	unsigned flags;
	int16_t code;
};
```
> The size includes the header and the trailing contents.  See the
> `gate_send_packet` function documentation about flags.
>
> Non-negative codes identify discovered services.  Negative codes are for
> built-in functionality.
>
> Sending a packet with code `GATE_PACKET_CODE_NOTHING` causes the flags to be
> processed by the runtime, but has no other effect.  Likewise, the runtime may
> send a packet with that code if it needs to send a notification, but has no
> queued packet to piggy-back it on.
>
> The size of the unsigned integer type of the flags field is unspecified.  The
> struct declaration may contain additional reserved fields which must be
> zeroed in sent packets.


### Service discovery

```c
struct gate_service_name_packet {
	struct gate_packet header;
	uint16_t count;
	char names[0]; // variable length
};
```
> Service discovery request, sent with the `GATE_PACKET_CODE_SERVICES` code.
> The count indicates how many nul-terminated service names are concatenated in
> *names*.  The *infos* array in the response packet will be in the same order
> as *names*.
>
> Services may be discovered in multiple steps.
>
> The struct declaration may contain additional reserved fields which must be
> zeroed.


```c
struct gate_service_info_packet {
	struct gate_packet header;
	uint16_t count;
	struct gate_service_info infos[0]; // variable length
};
```
> Service discovery response, received with the `GATE_PACKET_CODE_SERVICES`
> code.  It matches a service discovery request.  The count is the total number
> of discovered services, and *infos* is a concatenation of all previously and
> newly discovered service information.


```c
struct gate_service_info {
	unsigned flags;
	int32_t version;
};
```
> If the `GATE_SERVICE_FLAG_AVAILABLE` flag is set, packets may be sent to the
> service.  The packet code of the service is its index in the *infos* array
> (its discovery ordering).
>
> Semantics of the version are service-specific.
>
> Services won't start sending packets to the program (or allocate significant
> resources) until the program initiates interaction, so it's safe to discover
> services even if the program is unsure which ones it will actually use.
>
> The size of the unsigned integer type of the flags field is unspecified.


## Toolchain

[LLVM](https://llvm.org) and [clang](https://clang.llvm.org) have preliminary
support for WebAssembly.
The [wag-toolchain](https://github.com/tsavola/wag-toolchain) repository ties
them together with other tools, and provides scripts for building C and C++
programs which work with Gate.

The [gate-toolchain](https://hub.docker.com/r/tsavola/gate-toolchain) Docker image
is built on [wag-toolchain](https://hub.docker.com/r/tsavola/wag-toolchain) and contains
partial [musl](https://www.musl-libc.org) C standard library,
[dlmalloc](http://g.oswego.edu/dl/html/malloc.html), and the Gate API headers.
It's currently the easiest way to build C programs for Gate.  C++ can also be
used, but there is no C++ library support.

When building your program, sources must first be compiled into LLVM bitcode
files, which are then linked into a WebAssembly binary.  The sources of musl
and malloc libraries have been compiled into individual LLVM bitcode files,
found in the `/lib/wasm32` directory inside the Docker image.  When using the
libraries, all necessary library functions need to be manually linked to the
program.

When invoking the Docker image, the container should run with your credentials
and have access to the working directory.  Like this:

```sh
docker run -i --rm -u $(id -u):$(id -g) -v $PWD:$PWD -w $PWD tsavola/gate-toolchain ...
```

It contains the `compile`, `compile++` and `link` scripts.  They can be invoked
more or less like gcc.  For example:

```sh
docker ... tsavola/gate-toolchain compile -Wall -o example.bc example.c
docker ... tsavola/gate-toolchain link -o example.wasm example.bc /lib/wasm32/malloc.bc
```

There's also [example.c](examples/toolchain/example.c)
and its [Makefile](examples/toolchain/Makefile).
Build and run it with `make check-toolchain`.

