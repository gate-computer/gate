# Application binary interface for user programs


### Functions

ABI functions are accessed using WebAssembly module's import mechanism.

```wasm
(import "gate" "debug" (func (param i32) (param i32)))
```
> Write UTF-8 text to debug log, if enabled.  First argument is buffer pointer
> and second is length.
>
> Code point 0x1e is interpreted as optional newline: it ensures that text
> following it will be at the start of a line.


```wasm
(import "gate" "exit" (func (param i32)))
```
> Terminate the program, indicating that execution must not be resumed later.
> Parameter value 0 indicates success and 1 indicates failure.  Other values
> are interpreted as 1.


```wasm
(import "gate" "io.65536" (func (param i32) (param i32) (param i32) (param i32) (param i32)))
```
> Receive and/or send packet data.  First and second parameters are receive
> buffer and length.  Third and fourth parameters are send buffer and length.
> Fifth parameter is flags.
>
> A length is specified as pointer to 32-bit integer, which is updated to
> reflect the number of bytes transferred.  Specifying zero length or a null
> pointer disables transfer.
>
> A buffer might contain partial packet, a whole packet, or (parts of) multiple
> packets.
>
> The call is non-blocking by default.  Blocking behavior can be requested by
> specifying receive buffer and setting bit 1 in flags.  The call may still be
> interrupted without any bytes having been transferred.  Blocking send without
> receive is not supported.


```wasm
(import "gate" "randomseed" (func (result i64)))
```
> Return a cryptographically secure pseudorandom number.  If called multiple
> times, a different number may or may not be returned.


```wasm
(import "gate" "time" (func (param i32) (param i32) (result i32)))
```
> Get current wall-clock or monotonic time.  The first parameter identifies the
> clock: 0 means realtime and 1 monotonic time.  The second parameter is a
> pointer to a 16-byte target buffer: second and nanosecond values are stored
> into it as 64-bit integers.  The nanosecond value will be in range
> [0,999999999].  Actual resolution is unspecified.
>
> On success 0 is returned.  If an unsupported clock id is specified, -1 is
> returned.


### Packets

The I/O function exchanges packets with the runtime.  Packets have a 8-byte
header, followed by contents.  Maximum packet size is 65536 bytes.

Packet header consists of little-endian integer fields:

  1. 32-bit size (including this 8-byte header)
  2. 16-bit code
  3. 8-bit domain
  4. 8 reserved bits which must be zero in sent packets

Codes:

  - Non-negative codes are used for dynamically discovered services.
  - Code -1 is used for service discovery.
  - Other negative codes are reserved.  Packet with one must be ignored.

Domains:

  - 0 - Function calls.  Used to send requests and receive responses.
  - 1 - State change notifications.  Used by services to notify the program
        about things which don't require flow control.
  - 2 - Flow control.  Used to indicate how much data can be received per
        stream.
  - 3 - Data transfers.  Used to stream data according to flow control.

See [C API](C.md) documentation for descriptions of built-in packet types.

