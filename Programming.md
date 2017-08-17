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
binaries built with [binaryen](https://github.com/WebAssembly/binaryen) don't work.
Everything has to be built around the import functions provided by Gate.

[LLVM](https://llvm.org) and [clang](https://clang.llvm.org) have preliminary
support for WebAssembly.
The [wag-toolchain](https://github.com/tsavola/wag-toolchain) repository ties
them together with other tools, and provides scripts for building C and C++
programs which work with Gate.


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
size_t gate_recv_packet(void *buf, size_t size, unsigned int flags);
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
	uint16_t flags;
	uint16_t code;
};
```
> The size includes the header and the trailing contents.  See the
> `gate_send_packet` function documentation about flags.
>
> The code identifies a service.  Code 0 identifies the service discovery
> service.  It is the only valid code until more services have been discovered.
>
> As a special case, it's possible to send an empty packet with code 0 (empty
> means that its size equals the header size).  It causes the flags to be
> processed by the runtime, but it doesn't trigger a service discovery.
> Likewise, the runtime may send a empty packet with code 0 if it needs to send
> a notification, but has no queued packet to piggy-back it on.


### Service discovery

```c
struct gate_service_name_packet {
	struct gate_packet header;
	// reserved fields
	uint32_t count;
	char names[0]; // variable length
};
```
> Service discovery request, sent with code 0.  The count indicates how many
> nul-terminated service names are concatenated in *names*.


```c
struct gate_service_info_packet {
	struct gate_packet header;
	// reserved fields
	uint32_t count;
	struct gate_service_info infos[0]; // variable length
};
```
> Service discovery response, received with code 0.  It matches a service
> discovery request; the count is the same as in the request, and *infos* is in
> the same order as *names* in the request.
>
> Note that an empty packet with code 0 is not a service discovery response.


```c
struct gate_service_info {
	uint16_t code;
	// reserved fields
	int32_t version;
};
```
> If the code is non-zero, it can be used to send packets to the service and to
> identify packets received from the service.  Code 0 means that the service is
> not supported by the runtime.
>
> Semantics of the version are service-specific.
>
> Services won't start interacting with the program (or allocate significant
> resources) until the program does, so it's safe to discover services even if
> the program is unsure which ones it will actually use.  The contents of the
> packets are entirely service-specific.
>
> The code is only valid within the program instance which discovered it.


## Toolchain

TODO

