# Service implementation

## Naming

Service name can technically be any byte string which doesn't contain zero
bytes, but should be an UTF-8 string, preferrably using only the ASCII string.

Built-in service names never contain dots before the first slash, if any.
(Current built-in services container neither dots nor slashes.)  That naming
convention avoids conflicts with other common conventions:

  1. `net.example.service` (Java package)
  2. `net.example.Service` (Java class, D-Bus service)
  3. `example.net/service` (Go package)

Those conventions don't conflict with each other either.  Any one of them may
be used, as long as the domain name is controlled by the service author.


## Registry

When a program sends a [service discovery](Programming.md#service-discovery)
packet, the runtime asks a
[ServiceRegistry](https://godoc.org/github.com/tsavola/gate/run#ServiceRegistry)
implementation to look up the service names, and assigns codes for the ones
which are found.  When the program sends a packet to a service, the packet is
forwarded to the ServiceRegistry implementation to be handled.

The default
[ServiceRegistry implementation](https://godoc.org/github.com/tsavola/gate/service#Registry)
multiplexes packets to
[service implementations](https://godoc.org/github.com/tsavola/gate/service#Factory).
State management is completely service-specific.  If each program instance
requires distinct configuration for a given service, a modified ServiceRegistry
with a distinct Factory instance must be used for each program instance.

