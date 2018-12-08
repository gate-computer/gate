// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package webapi contains definitions useful for accessing the HTTP and
// websocket APIs.  See https://github.com/tsavola/gate/blob/master/Web.md for
// general documentation.
package webapi

import (
	"crypto"
	"encoding/base64"
	"encoding/json"
	"regexp"

	"github.com/tsavola/gate/internal/serverapi"
)

// Version of the Gate webserver API.
const Version = 0

// Name of the module reference source and associated content hash algorithm.
const ModuleRefSource = "sha384"

// Algorithm for converting module content to reference key.  A reference key
// string can be formed by encoding a hash digest with base64.RawURLEncoding.
const ModuleRefHash = crypto.SHA384

// Request URL paths.
const (
	Path           = "/v0"                               // The API.
	PathModule     = Path + "/module"                    // Base of relative module URIs.
	PathModules    = PathModule + "/"                    // Module sources.
	PathModuleRefs = PathModules + ModuleRefSource + "/" // Module reference keys.
	PathInstances  = Path + "/instance/"                 // Instance ids.
)

// Query parameters for post and websocket requests.
const (
	ParamAction = "action"
)

// Query parameters for ActionCall and ActionLaunch requests.
const (
	ParamFunction = "function"
	ParamInstance = "instance"
)

// Actions on modules (references and other sources).
const (
	ActionCall   = "call"   // Post or websocket.
	ActionLaunch = "launch" // Post.
)

// Actions on module references.
const (
	ActionUnref = "unref" // Post.
)

// Actions on instances.
const (
	ActionIO      = "io"      // Post or websocket.
	ActionStatus  = "status"  // Post.
	ActionSuspend = "suspend" // Post.
)

// HTTP request headers.
const (
	HeaderAuthorization = "Authorization" // "Bearer" JSON Web Token.
)

// HTTP request or response headers.
const (
	HeaderContentLength = "Content-Length"
	HeaderContentType   = "Content-Type"
)

// HTTP response headers.
const (
	HeaderLocation = "Location"        // Absolute module ref path.
	HeaderInstance = "X-Gate-Instance" // UUID.
	HeaderStatus   = "X-Gate-Status"   // Status of instance as JSON.
)

// The supported module content type.
const ContentTypeWebAssembly = "application/wasm"

// The supported key type.
const KeyTypeOctetKeyPair = "OKP"

// The supported elliptic curve.
const KeyCurveEd25519 = "Ed25519"

// The supported signature algorithm.
const SignAlgEdDSA = "EdDSA"

// The supported authorization type.
const AuthorizationTypeBearer = "Bearer"

// JSON Web Key.
type PublicKey struct {
	Kty string `json:"kty"`           // Key type.
	Crv string `json:"crv,omitempty"` // Elliptic curve.
	X   string `json:"x,omitempty"`   // Base64url-encoded unpadded public key.
}

// PublicKeyEd25519 creates a JWK for a JWT header.
func PublicKeyEd25519(publicKey []byte) *PublicKey {
	return &PublicKey{
		Kty: KeyTypeOctetKeyPair,
		Crv: KeyCurveEd25519,
		X:   base64.RawURLEncoding.EncodeToString(publicKey),
	}
}

// JSON Web Token header.
type TokenHeader struct {
	Alg string     `json:"alg"`           // Signature algorithm.
	JWK *PublicKey `json:"jwk,omitempty"` // Public side of signing key.
}

// TokenHeaderEdDSA creates a JWT header.
func TokenHeaderEdDSA(publicKey *PublicKey) *TokenHeader {
	return &TokenHeader{
		Alg: SignAlgEdDSA,
		JWK: publicKey,
	}
}

// MustEncode to a JWT component.
func (header *TokenHeader) MustEncode() []byte {
	serialized, err := json.Marshal(header)
	if err != nil {
		panic(err)
	}

	encoded := make([]byte, base64.RawURLEncoding.EncodedLen(len(serialized)))
	base64.RawURLEncoding.Encode(encoded, serialized)
	return encoded
}

// JSON Web Token payload.
type Claims struct {
	Exp   int64    `json:"exp"`             // Expiration time.
	Aud   []string `json:"aud,omitempty"`   // https://authority/api
	Nonce string   `json:"nonce,omitempty"` // Unique during expiration period.
}

// Status response header.
type Status struct {
	State string `json:"state,omitempty"`
	Cause string `json:"cause,omitempty"`
	Trap  string `json:"trap,omitempty"`
	Exit  int    `json:"exit,omitempty"`
	Error string `json:"error,omitempty"`
}

// Response to PathModuleRefs request.
type ModuleRefs struct {
	Modules []ModuleRef `json:"modules"`
}

// An item in a ModuleRefs response.
type ModuleRef = serverapi.ModuleRef

// Response to a PathInstances request.
type Instances struct {
	Instances []InstanceStatus `json:"instances"`
}

// An item in an Instances response.
type InstanceStatus struct {
	Instance string `json:"instance"`
	Status   Status `json:"status"`
}

// ActionCall websocket request message.
type Call struct {
	Authorization string `json:"authorization,omitempty"`
	ContentType   string `json:"content_type,omitempty"`
	ContentLength int64  `json:"content_length,omitempty"`
}

// Reply to Call message.
type CallConnection struct {
	Location string `json:"location,omitempty"` // Absolute module ref path.
	Instance string `json:"instance,omitempty"` // UUID.
}

// ActionIO websocket request message.
type IO struct {
	Authorization string `json:"authorization"`
}

// Reply to IO message.
type IOConnection struct {
	Connected bool   `json:"connected"`
	Status    Status `json:"status,omitempty"` // Instance status when not connected.
}

// Second and final text message on successful ActionCall or ActionIO websocket
// connection.
type ConnectionStatus struct {
	Status Status `json:"status"` // Instance status after disconnection.
}

// FunctionRegexp matches valid a function name.
var FunctionRegexp = regexp.MustCompile("^[A-Za-z0-9-._]+$")
