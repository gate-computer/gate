# Application binary interface for user programs

Gate implements [WASI](https://wasi.dev) so that toolchains can be used without
modification.  However, most functions are little more than stubs.  The
relevant ones are documented below.

Additional functions are provided for Gate-specific programs.  Importing one of
them causes the maximum size of received packets to be negotiated at load time;
the symbols are suffixed with a number which can be chosen by the program (but
must be at least 65536).

Since all WASI symbols are available, a binary can simultaneously target Gate
and other runtimes.  Gate sets the following environment variables:

- `GATE_ABI_VERSION` (currently 0)
- `GATE_FD` (a file descriptor number)
- `GATE_MAX_SEND_SIZE` (at least 65536)

Some WASI calls cannot be easily supported by Gate.  To avoid breaking API
contracts (or introducing unwanted complexity), such calls suspend the program.


### Functions

ABI functions are accessed using WebAssembly module's import mechanism.  The
functions are also exported in the `env` namespace, with `__gate_` or `__wasi_`
prefix.


```wasm
(import "gate" "fd_N" (func (result i32)))
```
> Get a non-blocking file descriptor for transferring packets using WASI
> functions `fd_read`, `fd_write` and `poll_oneoff`.
>
> `N` is the maximum packet size received through the file descriptor.
>
> The file descriptor number can also be discovered via environment variable
> `GATE_FD` to avoid creating a static dependency to Gate.


```wasm
(import "gate" "io_N" (func (param i32 i32 i32 i32 i32 i32 i64 i32)))
```
> Receive and/or send packet data.  (This is an alternative to the WASI I/O
> functions.)  Parameters 1-3 and 4-6 specify reception and sending parameters,
> respectively: I/O vector, I/O vector length, and pointer to a buffer for a
> 32-bit integer for storing the transferred size.  Parameter 7 is timeout in
> nanoseconds.
>
> I/O vectors consist of 8-byte entries.  An entry contains two 32-bit fields:
> pointer and length.
>
> A packet is padded so that its buffer size is rounded up to the next multiple
> of 8.  When sending a packet, 0-7 padding bytes must be sent after the packet
> to ensure alignment.
>
> `N` is the maximum size of received packets.
>
> TODO: document timeout and flags parameters
>
> The call may still be interrupted without any bytes having been transferred.


```wasm
(import "wasi_snapshot_preview1" "clock_time_get" (func (param i32 i32 i32) (result i32)))
```
> Get current wall-clock or monotonic time.  The first parameter identifies the
> clock: 0 means realtime and 1 monotonic time.  The second parameter is
> preferred time resolution in nanoseconds.  The actual resolution of the
> result is unspecified.  The third parameter is a pointer to a buffer for a
> 64-bit integer, where the time is stored in nanoseconds.
>
> On success 0 is returned.  Some other value is returned if invalid arguments
> are specified.


```wasm
(import "wasi_snapshot_preview1" "fd_read" (func (param i32 i32 i32 i32) (result i32)))
```
> Receive packet data through the Gate file descriptor.  Similar to `fd_write`.


```wasm
(import "wasi_snapshot_preview1" "fd_write" (func (param i32 i32 i32 i32) (result i32)))
```
> Write debug messages through file descriptor 1 or 2, or send packet data
> through the Gate file descriptor.  The first parameter is the file
> descriptor, the second is an I/O vector, the third is the I/O vector length,
> and the fourth is a pointer to a buffer for a 32-bit integer where the
> written size will be stored.
>
> The I/O vector consists of 8-byte entries.  An entry contains two 32-bit
> fields: pointer and length.
>
> On success 0 is returned.  Writing to file descriptors 1 and 2 always
> succeeds.  If writing to the Gate file descriptor would block, 6 (EAGAIN) is
> returned.  Some other value is returned if invalid arguments are specified.


```wasm
(import "wasi_snapshot_preview1" "poll_oneoff" (func (param i32 i32 i32 i32) (result i32)))
```
> Wait for I/O-readiness of the Gate file descriptor.  Note that waiting only
> for writability may lead to a deadlock.   Polling the Gate file descriptor
> always succeeds.
>
> See [WASI API](https://github.com/CraneStation/wasmtime/blob/master/docs/WASI-api.md#__wasi_poll_oneoff) for details.


```wasm
(import "wasi_snapshot_preview1" "proc_exit" (func (param i32)))
```
> Terminate the program, indicating that execution must not be resumed later.
> Parameter value 0 indicates success and 1 indicates failure.  Other values
> are interpreted as 1.


```wasm
(import "wasi_snapshot_preview1" "random_get" (func (param i32 i32) (result i32)))
```
> Get cryptographically secure pseudorandom data.  The first parameter is the
> buffer address and the second is the buffer length.
>
> This function can be used during program startup to seed a pseudorandom
> number generator, initialize hash tables etc.  But if more than 16 bytes is
> requested (cumulatively), the program may be terminated.
>
> The return value is always 0.


### Packets

The I/O functions exchange packets with the runtime.  Packets have a 8-byte
header, followed by contents.  Maximum size of a received packet is 65536
unless a larger value has been negotiated.  Maximum size of a sent packet is
65536 unless a larger limit has been discovered via environment variable
`GATE_MAX_SEND_SIZE`.

Packet header consists of little-endian integer fields:

  1. 32-bit size (including this 8-byte header; excluding padding)
  2. 16-bit code
  3. 8-bit domain
  4. 8-bit index

Codes:

  - Non-negative codes are used for dynamically discovered services.
  - Code -1 is used for service discovery.
  - Other negative codes are reserved.  Packet with one must be ignored.

Domains:

  - 0 - Function calls.  Used to send requests and receive responses.
  - 1 - Service information.  Used to notify the program about state changes
        asynchronously, without flow control.
  - 2 - Flow control.  Used to indicate how much data can be received per
        stream.
  - 3 - Data transfers.  Used to stream data according to flow control.

Index:

  - Must be zero in sent packets.
  - Must be ignored unless domain is 0 (function call).
  - See the C API for more information.

See [C API](c-api.md) documentation for descriptions of built-in packet types.

