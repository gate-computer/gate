# Web server API

This document is very much unfinished.

The [Go webapi package](https://godoc.org/github.com/tsavola/gate/webapi) has
definitions useful for clients.


## Authorization header

A `Bearer` JSON Web Token is required for module and instance API requests.
It's not required for requests which query general information.

The JWT must identify a principal by specifying an Ed25519 public key via the
"jwk" header.  See the PublicKey struct for details.  The JWT must be signed
using the EdDSA algorithm ("alg" header).

Expiration time ("exp" claim) is checked by the server so that it won't be too
far in the future.  The limit is 15 minutes.

The "nonce" claim may be specified in order to prevent token reuse.  If set, it
must be unique during the expiration period.

The "aud" claim may be specified in order to prevent misdirected requests.
Suitable audience string is the HTTPS URL of the API,
e.g. `https://gate.example.net/v0` or `https://example.net/gate/v0`.


## Function name

Function strings consist of ASCII letters, digits, dash, dot and underscore.


## Instance identifier

Instance strings are UUIDs; RFC 4122 format, version 4 (random).  When creating
an instance, an explicit identifier should not be specified unless migrating an
existing instance from another host.
