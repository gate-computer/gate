# Application binary interface


#### Functions

ABI functions are accessed using WebAssembly module's import mechanism.

```wasm
(import "env" "__gate_debug_write" (func (param i32) (param i32)))
```
> Output debug data.  The first argument is buffer pointer and the second is
> length.


```wasm
(import "env" "__gate_exit" (func (param i32)))
```
> Terminate the program.  The parameter value 0 indicates success and 1
> indicates failure.  Other values are reserved.


```wasm
(import "env" "__gate_func_ptr" (func (param i32) (result i32)))
```
> Get a pointer to a function.  The parameter is a function id.  If an
> unsupported id is specified, null is returned.


```wasm
(import "env" "__gate_get_abi_version" (func (result i32)))
```
> Get the runtime ABI version.


```wasm
(import "env" "__gate_get_arg" (func (result i32)))
```
> Get the argument.


```wasm
(import "env" "__gate_get_max_packet_size" (func (result i32)))
```
> Find out how large I/O packets the runtime supports.  It's a fatal error to
> send a larger packet.


```wasm
(import "env" "__gate_recv" (func (param i32) (param i32) (param i32) (result i32)))
```
> Receive part of a packet into a buffer.  The first parameter is buffer
> pointer, the second is length, and the third contains flags.
>
> If flags is 0, the call blocks until *length* bytes have been received into
> the buffer.  The return value is unspecified.
>
> If flags is 1, the call doesn't block, but all bytes might not be received.
> The return value indicates the number of remaining bytes (length minus the
> received bytes).
>
> Other flags are reserved.


```wasm
(import "env" "__gate_send" (func (param i32) (param i32)))
```
> Send part of a packet.  The first parameter is buffer pointer and the second
> is length.  The call might block depending on I/O state.


#### Packets

The `__gate_recv` and `__gate_send` functions exchange packets with the
runtime.  Packets have a 8-byte header, followed by contents.

Packet header consists of little-endian integer fields (without padding):

  1. 32-bit size - including the header
  2. 16-bit flags - value 1 indicates *pollout* flag, other values are reserved
  3. 16-bit code

The [Programming interface](Programming.md) documents the semantics.

