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
GATE_DEBUG_SEPARATOR
```
> A string which can be used in debug log messages.  It will start a new line
> unless the write position is already at the start of a line.


```c
GATE_IO_RECV_WAIT
```
> I/O function flag.


```c
GATE_MAX_PACKET_SIZE
```
> Packet size limit.


```c
GATE_PACKET_CODE_SERVICES
```
> Packet code for service discovery.


```c
GATE_PACKET_DOMAIN_CALL
GATE_PACKET_DOMAIN_STATE
GATE_PACKET_DOMAIN_FLOW
GATE_PACKET_DOMAIN_DATA
```
> Packet domains.


```c
GATE_SERVICE_STATE_AVAIL
```
> Service state flag.


#### Functions

```c
void gate_debug(arg)
void gate_debug1(arg)
void gate_debug2(arg, arg)
void gate_debug3(arg, arg, arg)
void gate_debug4(arg, arg, arg, arg)
void gate_debug5(arg, arg, arg, arg, arg)
void gate_debug6(arg, arg, arg, arg, arg, arg)
```
> Write to the debug log if enabled by runtime and `NDEBUG` wasn't defined
> during compilation.  The arguments can be integers, strings or void-pointers.
> No implicit delimiters (spaces or newlines) are written.


```c
void gate_debug_int(int64_t n)
void gate_debug_uint(uint64_t n)
```
> Write a decimal number to the debug log if enabled by runtime and `NDEBUG`
> wasn't defined during compilation.


```c
void gate_debug_hex(uint64_t n)
void gate_debug_ptr(const void *ptr)
```
> Write a hexadecimal number to the debug log if enabled by runtime and
> `NDEBUG` wasn't defined during compilation.  The "ptr" variant writes "0x"
> before the number.


```c
void gate_debug_str(const char *s)
void gate_debug_data(const char *data, size_t size)
```
> Write a UTF-8 string to the debug log if enabled by runtime and `NDEBUG`
> wasn't defined during compilation.


```c
void gate_exit(int status)
```
> Terminate the program.  Status 0 indicates success and 1 indicates failure.
> Other values are interpreted as 1.


```c
void gate_io(void * restrict recv_buffer, size_t * restrict recv_length,
             const void * restrict send_buffer, size_t * restrict send_length,
			 unsigned io_flags)
```
> Receive and/or send packet data.
>
> Receive and send lengths are specified as pointers to integers, which are
> updated to reflect the number of bytes transferred.  Specifying zero length
> or a null pointer disables transfer.
>
> A buffer might contain partial packet, a whole packet, or (parts of) multiple
> packets.
>
> The call is non-blocking by default.  Blocking behavior can be requested by
> specifying receive buffer and `GATE_IO_RECV_WAIT` flag.  The call may still
> be interrupted without any bytes having been transferred.  Blocking send
> without receive is not supported.



#### Packet header

```c
struct gate_packet {
	uint32_t size;
	int16_t code;
	uint8_t domain;
}
```
> The size includes the header and the trailing contents.
>
> Non-negative codes identify discovered services.  Negative codes are for
> built-in functionality.
>
> The `GATE_PACKET_DOMAIN_CALL` domain is used for sending requests to
> services.  Each request is matched with one response.  The responses are
> received in the same order as requests are sent (per service).
>
> The `GATE_PACKET_DOMAIN_STATE` domain is for receiving state change
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
}
```
> Service discovery request, sent with the `GATE_PACKET_CODE_SERVICES` code.
> *count* indicates how many nul-terminated service names are concatenated in
> *names*.  The *states* array in the response packet will be in the same order
> as *names*.
>
> Services may be discovered in multiple steps.
>
> The struct declaration may contain additional reserved fields which must be
> zeroed.


```c
struct gate_service_state_packet {
	struct gate_packet header;
	uint16_t count;
	uint8_t states[0]; // Variable length.
}
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
> service, transmission of the packet can be completed.  In other words, up to
> `GATE_MAX_PACKET_SIZE` bytes can be sent to disappeared services.  It is a
> fatal error to send more.  No data can be sent to services which haven't been
> available.


### Streaming

Services may provide uni- or bi-directional byte streams.  They may be
implicit, or opened explicitly via a call.  Either way, they are subject to
flow control and don't generate packets before permitted by the program.


```c
struct gate_flow_packet {
	struct gate_packet header;
	struct gate_flow flows[0]; // Variable length.
}
```
> Reception capacity notification for one or more streams.  All streams belong
> to the service identified by the code in the packet header.


```c
struct gate_flow {
	int32_t id;
	uint32_t increment;
}
```
> Indicates that the reception capacity of the stream identified by *id* has
> increased by *increment* bytes.


```c
struct gate_data_packet {
	struct gate_packet header;
	int32_t id;
	char data[0]; // Variable length.
}
```
> Data transfer for the stream identified by *id*, which belongs to the service
> identified by the code in the packet header.  The length of *data* is
> implicitly decremented from the reception capacity.
>
> The struct declaration may contain additional reserved fields which must be
> zeroed in sent packets.

