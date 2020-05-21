# Web server API

This document is very much unfinished.

The [Go webapi package](https://godoc.org/gate.computer/gate/webapi) has
definitions useful for clients.


## Authorization header

A `Bearer` JSON Web Token is required for most module and instance API
requests.  It's not required for requests which query general information, or
if server configuration allows anonymous access to a module action.

The JWT must identify a principal by specifying an Ed25519 public key via the
`jwk` header.  See the [PublicKey](https://godoc.org/gate.computer/gate/webapi#PublicKey)
struct for details.  The JWT must be signed using the EdDSA algorithm (`alg`
header).

Expiration time (`exp` claim) is checked by the server so that it won't be too
far in the future.  The limit is 15 minutes.

The `nonce` claim may be specified in order to prevent token reuse.  If set, it
must be unique during the expiration period.  Server configuration may preclude
nonce usage.

The `aud` claim may be specified in order to prevent misdirected requests.  The
audience string is the HTTPS URL of the API, e.g. `https://example.net/gate-0/`.


## Function name

Function strings consist of ASCII letters, digits, dash, dot and underscore.
The length must be between 1 and 31 characters (inclusive).


## Instance identifier

Instance strings are UUIDs; RFC 4122 format, version 4 (random).  When creating
an instance, an explicit identifier should not be specified unless migrating an
existing instance from another host.


## Feature detection

It's safe to query information using GET and HEAD requests at any directory
path (ending with a slash).  The server responds with HTTP status 405 (Method
Not Allowed) if the HTTP method is not supported (yet) for the given resource.

The server responds with HTTP status 501 (Not Implemented) if the `action`
query parameter value is unknown.  New actions may be added at any time, so
server deployments might not support all features.  (Semantics of existing
actions don't change for a given resource location.)

