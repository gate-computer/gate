# Service implementation

See
[ServiceRegistry](https://godoc.org/gate.computer/gate/runtime#ServiceRegistry),
[service package](https://godoc.org/gate.computer/gate/service) and
[C API](C.md#packet-header) documentation.


## Naming

Service names are valid UTF-8 strings consisting of letter, number and
punctuation characters.  Encoded length must be between 1 and 127 bytes
(inclusive).

Built-in service names are guaranteed to never contain dashes, dots or colons
before the first slash (if any).  That convention avoids conflicts with other
common naming conventions:

  1. `example.net/service` (Go package)
  2. `net.example.service` (Java package)
  3. `net.example.Service` (Java class, D-Bus service)
  4. `123e4567-e89b-12d3-a456-426655440000` (UUID)
  5. `https://example.net/service` (URL)

Those conventions don't conflict with each other either.  Any one of them may
be used, as long as the domain name is controlled by the service author or the
UUID is properly randomized.


## Call domain

Calls should be answered in order to not leave the caller hanging.  A
convention for handling unsupported calls is to reply with an empty packet
(nothing but the header), and make sure that an empty packet is never a
successful response to a supported call.

Services which don't implement any calls (yet) may choose to not answer them
(yet), if that seems more appropriate.  But please note that in such a case
programs cannot detect unsupported calls.


## Error codes

Packets have no predefined error codes.  Call and info packets have no
predefined error fields, but since their content is service-specific, any error
encoding is possible.  An error can be encoded in a flow packet's increment
field as a negative 32-bit integer, and in a data packet's note field as any
32-bit integer.  So when designing an error code scheme for a service, it
should be noted that error conditions which are common to all domains must be
expressible as negative 32-bit integers.

