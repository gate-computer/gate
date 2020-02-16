# Low-level C API for user programs

```c
#include <gate.h>
```
> The header can be found in the include directory (Gate repository top level).


### Primitives

#### Compile-time definitions

```c
GATE_API_VERSION
```
> The C API version.  The current version is 0.


```c
GATE_IO_WAIT
```
> I/O flag.


```c
GATE_PACKET_ALIGNMENT
```
> Granularity of I/O packet buffers.


```c
GATE_MAX_RECV_SIZE
```
> Size limit for received packets.


```c
GATE_PACKET_CODE_SERVICES
```
> Packet code for service discovery.


```c
GATE_PACKET_DOMAIN_CALL
GATE_PACKET_DOMAIN_INFO
GATE_PACKET_DOMAIN_FLOW
GATE_PACKET_DOMAIN_DATA
```
> Packet domains.


```c
GATE_SERVICE_STATE_AVAIL
```
> Service state flag.


#### Macros

```c
GATE_ALIGN_PACKET(size)
```
> Rounds packet size up to a multiple of packet alignment.


#### Types

```c
struct gate_iovec {
	void *iov_base;
	size_t iov_len;
};
```
> Specifies buffers for scatter-gather I/O.


#### Functions

```c
uint64_t gate_clock_realtime(void);
uint64_t gate_clock_monotonic(void);
```
> Get current wall-clock or monotonic time in nanoseconds.  Actual resolution
> is unspecified.


```c
void gate_debug(arg);
void gate_debug1(arg);
void gate_debug2(arg, arg);
void gate_debug3(arg, arg, arg);
void gate_debug4(arg, arg, arg, arg);
void gate_debug5(arg, arg, arg, arg, arg);
void gate_debug6(arg, arg, arg, arg, arg, arg);
```
> Write to the debug log if enabled by runtime and `NDEBUG` wasn't defined
> during compilation.  The arguments can be integers, strings or void-pointers.
> No implicit delimiters (spaces or newlines) are written.


```c
void gate_debug_int(int64_t n);
void gate_debug_uint(uint64_t n);
```
> Write a decimal number to the debug log if enabled by runtime and `NDEBUG`
> wasn't defined during compilation.


```c
void gate_debug_hex(uint64_t n);
void gate_debug_ptr(const void *ptr);
```
> Write a hexadecimal number to the debug log if enabled by runtime and
> `NDEBUG` wasn't defined during compilation.  The "ptr" variant writes "0x"
> before the number.


```c
void gate_debug_str(const char *s);
void gate_debug_data(const char *data, size_t size);
```
> Write a UTF-8 string to the debug log if enabled by runtime and `NDEBUG`
> wasn't defined during compilation.


```c
void gate_exit(int status);
```
> Terminate the program, indicating that execution must not be resumed later.
> Status 0 indicates success and 1 indicates failure.  Other values are
> interpreted as 1.


```c
void gate_io(const struct gate_iovec *recv,
             int recvveclen,
             size_t *nreceived,
             const struct gate_iovec *send,
             int sendveclen,
             size_t *nsent,
             unsigned flags);
size_t gate_recv(void *buf, size_t size, unsigned flags);
size_t gate_send(const void *data, size_t size);
```
> Receive and/or send packet data.  The transferred data sizes are returned
> through the *nreceived* and *nsent* pointers, or as return values.
>
> A packet is padded so that its buffer size is rounded up to the next multiple
> of `GATE_PACKET_ALIGNMENT`.  When sending a packet, `0` to
> `GATE_PACKET_ALIGNMENT-1` padding bytes must be sent after the packet to
> ensure alignment.
>
> The call is non-blocking by default.  Blocking behavior can be requested by
> specifying the `GATE_IO_WAIT` flag.  The call may still be interrupted
> without any bytes having been transferred.


#### Packet header

```c
struct gate_packet {
	uint32_t size;
	int16_t code;
	uint8_t domain;
};
```
> The size includes the header and the trailing contents, but not the padding.
>
> Non-negative codes identify discovered services.  Negative codes are for
> built-in functionality.
>
> The `GATE_PACKET_DOMAIN_CALL` domain is used for sending requests to
> services.  Each request is matched with one response.  The responses are
> received in the same order as requests are sent (per service).
>
> The `GATE_PACKET_DOMAIN_INFO` domain is for receiving state change
> notifications from services.  A service won't start sending notifications
> before at least one call is made to that service.
>
> The struct declaration may contain additional reserved fields which must be
> zeroed in sent packets.


### Service discovery

```c
struct gate_service_name_packet {
	struct gate_packet header;
	uint16_t count;
	char names[0]; // Variable length.
};
```
> Service discovery request, sent with the `GATE_PACKET_CODE_SERVICES` code.
> *count* indicates how many service names are concatenated in *names*.  The
> *states* array in the response packet will be in the same order as *names*.
>
> Service names are encoded by prefixing each string with its length as a
> single byte.
>
> Services may be discovered in multiple steps.
>
> The struct declaration may contain additional reserved fields which must be
> zeroed in sent packets.


```c
struct gate_service_state_packet {
	struct gate_packet header;
	uint16_t count;
	uint8_t states[0]; // Variable length.
};
```
> Service discovery response or state change notification, received with
> `GATE_PACKET_CODE_SERVICES` code.   *count* is the total number of discovered
> services; *states* is a concatenation of all previously and newly discovered
> services.  A state item contains service state flags
> (`GATE_SERVICE_STATE_AVAIL`).
>
> If a call reply packet has a *count* which is less than the number of
> requested services, it means that the maximum service count has been reached.
>
> When the `GATE_SERVICE_STATE_AVAIL` flag is unset for a service, sending of
> packets to that service must cease.  If a partial packet has been sent to the
> service, transmission of the packet can be completed.  It is a fatal error to
> send more.  No data can be sent to services which haven't been available.


### Streaming

Services may provide uni- or bi-directional byte streams.  They may be
implicit, or opened explicitly via a call.  Either way, they are subject to
flow control and don't generate packets before permitted by the program.


```c
struct gate_flow_packet {
	struct gate_packet header;
	struct gate_flow flows[0]; // Variable length.
};
```
> Reception capacity notification for one or more streams.  All streams belong
> to the service identified by the code in the packet header.
>
> The struct declaration may contain additional reserved fields which must be
> zeroed in sent packets.


```c
struct gate_flow {
	int32_t id;
	int32_t increment;
};
```
> Indicates that the reception capacity of the stream identified by *id* has
> increased by *increment* bytes.
>
> The reception capacity must not exceed (2^31)-1 at any given time.


```c
struct gate_data_packet {
	struct gate_packet header;
	int32_t id;
	int32_t note;
	char data[0]; // Variable length.
};
```
> Data transfer for the stream identified by *id* which belongs to the service
> identified by the code in the packet header.  The length of *data* is
> implicitly decremented from the reception capacity.
>
> Note value's semantics are specified separately by each service.
>
> The struct declaration may contain additional reserved fields which must be
> zeroed in sent packets.

