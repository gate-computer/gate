# Service implementation

See the
[ServiceRegistry](https://godoc.org/github.com/tsavola/gate/runtime#ServiceRegistry)
and [service package](https://godoc.org/github.com/tsavola/gate/service)
documentation.


## Naming

Service name can technically be any byte string which doesn't contain zero
bytes, but should be an UTF-8 string, preferrably using only the ASCII subset.
Maximum name length is 127 bytes.

Built-in service names never contain dots before the first slash, if any.
(Current built-in services contain neither dots nor slashes.)  That naming
convention avoids conflicts with other common conventions:

  1. `example.net/service` (Go package)
  2. `net.example.service` (Java package)
  3. `net.example.Service` (Java class, D-Bus service)

Those conventions don't conflict with each other either.  Any one of them may
be used, as long as the domain name is controlled by the service author.

