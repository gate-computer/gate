# Service implementation

See
[ServiceRegistry](https://godoc.org/github.com/tsavola/gate/runtime#ServiceRegistry),
[service package](https://godoc.org/github.com/tsavola/gate/service) and
[C API](C.md#packet-header) documentation.


## Naming

Service names are valid UTF-8 strings consisting of letter, number and
punctuation characters.  Encoded length must be between 1 and 127 bytes
(inclusive).

Built-in service names never contain dots before the first slash, if any.  That
naming convention avoids conflicts with other common conventions:

  1. `example.net/service` (Go package)
  2. `net.example.service` (Java package)
  3. `net.example.Service` (Java class, D-Bus service)
  4. `123e4567-e89b-12d3-a456-426655440000` (UUID)

Those conventions don't conflict with each other either.  Any one of them may
be used, as long as the domain name is controlled by the service author or the
UUID is properly randomized.


## Call domain

Each call should be answered in order to not leave the caller hanging.  Even
services which don't implement any calls should answer them.  A convention for
handling unsupported calls is to reply with an empty call packet (nothing but
the header), and make sure that an empty packet is never a successful response
to a supported call.

