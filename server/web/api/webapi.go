// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package api contains definitions useful for accessing the HTTP and websocket
// APIs.  See https://github.com/gate-computer/gate/blob/master/Web.md for
// general documentation.
//
// This package avoids dependencies to the server implementation.
package api

import (
	"crypto"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
)

// KnownModuleSource is the name of the built-in directory of modules the
// content of which are known to the server and/or the client.
const KnownModuleSource = "sha256"

// KnownModuleHash algorithm for converting module content to its raw id within
// the KnownModuleSource.  The id string can be formed by encoding the hash
// digest with EncodeKnownModule.
const KnownModuleHash = crypto.SHA256

// EncodeKnownModule converts module content hash digest to its id within
// KnownModuleSource.  The input can be obtained using KnownModuleHash.
func EncodeKnownModule(hashSum []byte) string {
	return hex.EncodeToString(hashSum)
}

// Request URL paths.
const (
	Path              = "/gate-0/"              // The API.
	PathModule        = Path + "module"         // Base of relative module URIs.
	PathModuleSources = Path + "module/"        // Module source directory.
	PathKnownModules  = Path + "module/sha256/" // Known module directory.
	PathInstances     = Path + "instance/"      // Instance ids.
)

// Query parameters.
const (
	ParamAction      = "action"
	ParamModuleTag   = "module.tag"   // For pin or snapshot action.
	ParamFunction    = "function"     // For call, launch or resume action.
	ParamInstance    = "instance"     // For call or launch action.
	ParamInstanceTag = "instance.tag" // For call, launch or update action.
	ParamLog         = "log"          // For call, launch or resume action.
)

// Actions on modules.  ActionPin can be combined with ActionCall or
// ActionLaunch in a single request (ParamAction appears twice in the URL).
const (
	ActionPin    = "pin"    // Put (known), post (source) or websocket (call/launch).
	ActionUnpin  = "unpin"  // Post (known).
	ActionCall   = "call"   // Put (known), post (any) or websocket (any).
	ActionLaunch = "launch" // Put (known), post (any).
)

// Actions on instances.  ActionWait can be combined with ActionKill or
// ActionSuspend in a single request (ParamAction appears twice in the URL).
// ActionSuspend can be combined with ActionLaunch on a module: the instance
// will be created in StateSuspended or StateHalted.
const (
	ActionIO       = "io"       // Post or websocket.
	ActionWait     = "wait"     // Post.
	ActionKill     = "kill"     // Post.
	ActionSuspend  = "suspend"  // Post.
	ActionResume   = "resume"   // Post.
	ActionSnapshot = "snapshot" // Post.
	ActionDelete   = "delete"   // Post.
	ActionUpdate   = "update"   // Post.
	ActionDebug    = "debug"    // Post.  See the debug package.
)

// HTTP request headers.
const (
	HeaderAccept        = "Accept"
	HeaderAuthorization = "Authorization" // "Bearer" JSON Web Token.
	HeaderOrigin        = "Origin"
)

// HTTP request or response headers.
const (
	HeaderContentLength = "Content-Length"
	HeaderContentType   = "Content-Type"
)

// HTTP response headers.
const (
	HeaderLocation = "Location"      // Absolute path to known module.
	HeaderInstance = "Gate-Instance" // UUID.
	HeaderStatus   = "Gate-Status"   // Status of instance as JSON.
)

// The supported module content type.
const ContentTypeWebAssembly = "application/wasm"

// The supported instance update and debug content type.
const ContentTypeJSON = "application/json"

// The supported key type.
const KeyTypeOctetKeyPair = "OKP"

// The supported elliptic curve.
const KeyCurveEd25519 = "Ed25519"

// The supported signature algorithms.
const (
	SignAlgEdDSA = "EdDSA"
	SignAlgNone  = "none"
)

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
	Exp   int64    `json:"exp,omitempty"`   // Expiration time.
	Aud   []string `json:"aud,omitempty"`   // https://authority/api
	Nonce string   `json:"nonce,omitempty"` // Unique during expiration period.
	Scope string   `json:"scope,omitempty"`
}

// AuthorizationBearerEd25519 creates a signed JWT token (JWS).  TokenHeader
// must have been encoded beforehand.
func AuthorizationBearerEd25519(privateKey ed25519.PrivateKey, tokenHeader []byte, claims *Claims) (string, error) {
	b, err := unsignedBearer(tokenHeader, claims)
	if err != nil {
		return "", err
	}

	sig := ed25519.Sign(privateKey, b[len(AuthorizationTypeBearer)+1:len(b)-1])
	sigOff := len(b)
	b = b[:cap(b)]
	base64.RawURLEncoding.Encode(b[sigOff:], sig)
	return string(b), nil
}

// AuthorizationBearerLocal creates an unsecured JWT token.
func AuthorizationBearerLocal(claims *Claims) (string, error) {
	if claims == nil {
		claims = new(Claims)
	}

	header := (&TokenHeader{
		Alg: SignAlgNone,
	}).MustEncode()

	b, err := unsignedBearer(header, claims)
	return string(b), err
}

func unsignedBearer(header []byte, claims *Claims) ([]byte, error) {
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return nil, err
	}

	sigLen := base64.RawURLEncoding.EncodedLen(ed25519.SignatureSize)
	claimsLen := base64.RawURLEncoding.EncodedLen(len(claimsJSON))

	b := make([]byte, 0, len(AuthorizationTypeBearer)+1+len(header)+1+claimsLen+1+sigLen)
	b = append(b, (AuthorizationTypeBearer + " ")...)
	b = append(b, header...)
	b = append(b, '.')
	claimsOff := len(b)
	b = b[:claimsOff+claimsLen]
	base64.RawURLEncoding.Encode(b[claimsOff:], claimsJSON)
	b = append(b, '.')
	return b, nil
}

// Instance state enumeration.
const (
	StateRunning    = "RUNNING"
	StateSuspended  = "SUSPENDED"
	StateHalted     = "HALTED"
	StateTerminated = "TERMINATED"
	StateKilled     = "KILLED"
)

// Instance state cause enumeration.  Empty value means that the cause is a
// normal one (e.g. client action, successful completion).
//
// The cause enumeration is open-ended: new values may appear in the future.
const (
	CauseNormal = ""

	// Abnormal causes for StateSuspended:
	CauseCallStackExhausted = "CALL_STACK_EXHAUSTED"
	CauseABIDeficiency      = "ABI_DEFICIENCY"
	CauseBreakpoint         = "BREAKPOINT"

	// Abnormal causes for StateKilled:
	CauseUnreachable                   = "UNREACHABLE"
	CauseMemoryAccessOutOfBounds       = "MEMORY_ACCESS_OUT_OF_BOUNDS"
	CauseIndirectCallIndexOutOfBounds  = "INDIRECT_CALL_INDEX_OUT_OF_BOUNDS"
	CauseIndirectCallSignatureMismatch = "INDIRECT_CALL_SIGNATURE_MISMATCH"
	CauseIntegerDivideByZero           = "INTEGER_DIVIDE_BY_ZERO"
	CauseIntegerOverflow               = "INTEGER_OVERFLOW"
	CauseABIViolation                  = "ABI_VIOLATION"
	CauseInternal                      = "INTERNAL"
)

// Status response header.
type Status struct {
	State  string `json:"state,omitempty"`
	Cause  string `json:"cause,omitempty"`
	Result int    `json:"result,omitempty"` // Meaningful if StateHalted or StateTerminated.
	Error  string `json:"error,omitempty"`  // Optional details for abnormal causes.
}

func (status Status) String() (s string) {
	switch {
	case status.State == "":
		if status.Error == "" {
			return "error"
		} else {
			return fmt.Sprintf("error: %s", status.Error)
		}

	case status.Cause != "":
		s = fmt.Sprintf("%s abnormally: %s", status.State, status.Cause)

	case status.State == StateHalted || status.State == StateTerminated:
		s = fmt.Sprintf("%s with result %d", status.State, status.Result)

	default:
		s = status.State
	}

	if status.Error != "" {
		s = fmt.Sprintf("%s; error: %s", s, status.Error)
	}
	return
}

// Response to PathKnownModules request.
type Modules struct {
	Modules []ModuleInfo `json:"modules"`
}

// ModuleInfo 'r' mation.
type ModuleInfo struct {
	ID   string   `json:"id"`
	Tags []string `json:"tags,omitempty"`
}

// Response to a PathInstances request.
type Instances struct {
	Instances []InstanceInfo `json:"instances"`
}

// InstanceInfo 'r' mation.
type InstanceInfo struct {
	Instance  string   `json:"instance"`
	Module    string   `json:"module"`
	Status    Status   `json:"status"`
	Transient bool     `json:"transient,omitempty"`
	Debugging bool     `json:"debugging,omitempty"`
	Tags      []string `json:"tags,omitempty"`
}

// Instance update request content.
type InstanceUpdate struct {
	Persist bool     `json:"transient,omitempty"`
	Tags    []string `json:"tags,omitempty"`
}

// ActionCall websocket request message.
type Call struct {
	Authorization string `json:"authorization,omitempty"`
	ContentType   string `json:"content_type,omitempty"`
	ContentLength int64  `json:"content_length,omitempty"`
}

// Reply to Call message.
type CallConnection struct {
	Location string `json:"location,omitempty"` // Absolute path to known module.
	Instance string `json:"instance,omitempty"` // UUID.
}

// ActionIO websocket request message.
type IO struct {
	Authorization string `json:"authorization"`
}

// Reply to IO message.
type IOConnection struct {
	Connected bool `json:"connected"`
}

// Second and final text message on successful ActionCall or ActionIO websocket
// connection.
type ConnectionStatus struct {
	Status Status `json:"status"` // Instance status after disconnection.
}

// FunctionRegexp matches a valid function name.
var FunctionRegexp = regexp.MustCompile("^[A-Za-z0-9-._]{1,31}$")
