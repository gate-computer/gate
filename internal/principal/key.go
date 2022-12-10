// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package principal

import (
	"crypto/ed25519"
	"encoding/base64"
	"net/http"

	"gate.computer/internal/error/grpc"
)

const (
	keySize       = 32
	encodedKeyLen = 43
)

type Key struct {
	id ID
}

func ParseEd25519Key(encodedKey string) (*Key, error) {
	pri := &Key{ID{s: string(TypeEd25519) + ":" + encodedKey}}
	err := parseEd25519Key(pri.id.key[:], encodedKey)
	return pri, err
}

func parseEd25519Key(dest []byte, encodedKey string) error {
	if len(encodedKey) != encodedKeyLen {
		return principalKeyError("encoded principal key has wrong length")
	}

	n, err := base64.RawURLEncoding.Decode(dest, []byte(encodedKey))
	if err != nil {
		return principalKeyError("base64url encoding of principal key is invalid")
	}

	if n != len(dest) {
		return principalKeyError("decoded principal key has wrong length")
	}

	return nil
}

func (pri *Key) PrincipalID() *ID {
	return &pri.id
}

func (pri *Key) PublicKey() ed25519.PublicKey {
	return ed25519.PublicKey(pri.id.key[:])
}

type principalKeyError string

func (s principalKeyError) Error() string         { return string(s) }
func (s principalKeyError) PublicError() string   { return string(s) }
func (s principalKeyError) Unauthenticated() bool { return true }
func (s principalKeyError) Status() int           { return http.StatusUnauthorized }
func (s principalKeyError) GRPCCode() int         { return grpc.Unauthenticated }
