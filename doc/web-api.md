# Web server API

This document is very much unfinished.

OpenAPI definition:
https://github.com/gate-computer/gate/blob/main/openapi.yaml

The
[Go package gate.computer/gate/web](https://pkg.go.dev/gate.computer/gate/web)
has definitions useful for Go clients.


## Authorization header

A `Bearer` JSON Web Token is required for most module and instance API
requests.  It's not required for requests which query general information, or
if server configuration allows anonymous access to a module action.

The JWT must identify a principal by specifying an Ed25519 public key via the
`jwk` header.  See the
[PublicKey](https://pkg.go.dev/gate.computer/gate/web#PublicKey) struct for
details.  The JWT must be signed using the EdDSA algorithm (`alg` header).

Expiration time (`exp` claim) is checked by the server so that it won't be too
far in the future.  The limit is 15 minutes.

The `nonce` claim may be specified in order to prevent token reuse.  If set, it
must be unique during the expiration period.  Server configuration may preclude
nonce usage.

The `aud` claim may be specified in order to prevent misdirected requests.  The
audience string is the primary API URL, e.g. `https://example.net/gate-0/` or
`http://localhost:6473/gate-0/`.  The scheme is `https` (or `http`) also for
websocket connections.  Redirections don't affect the audience string: if the
example.net Gate API offloads processing to `api.example.net`, the audience
string will still have hostname `example.net`.


## Function name

Function strings consist of ASCII letters, digits, dash, dot and underscore.
The length must be between 1 and 31 characters (inclusive).


## Instance identifier

Instance strings are UUIDs; RFC 4122 format, version 4 (random).  When creating
an instance, an explicit identifier should not be specified unless migrating an
existing instance from another host.


## Feature detection

GET, HEAD and OPTIONS requests to any existing resource path succeed (if
sufficiently authenticated), even if the resource is only meaningful to access
using other methods.  It is safe to request information using GET and HEAD
methods without query parameters: it doesn't cause side-effects to any resource
(but it may spend quota or such, if authenticated); if the GET method is not
implemented for an otherwise valid path, the server responds with HTTP status
200 (OK) with no content type and zero content length.  OPTIONS indicates which
additional methods are implemented for the resource.  The server responds with
HTTP status 405 (Method Not Allowed) if the requested method is not implemented
for the resource.

The server responds with HTTP status 501 (Not Implemented) if the `action`
query parameter value is unknown.  New actions may be added at any time, so
server deployments might not support all features.  Semantics of existing
actions don't change for a given resource location.

