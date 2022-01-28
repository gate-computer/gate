// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package principal

import (
	"crypto/ed25519"
	"encoding/base64"
)

const (
	keySize       = 32
	encodedKeyLen = 43
)

type Key struct {
	id ID
}

func ParseEd25519Key(encodedKey string) (pri *Key, err error) {
	pri = &Key{ID{s: TypeEd25519 + ":" + encodedKey}}
	err = parseEd25519Key(pri.id.key[:], encodedKey)
	return
}

func parseEd25519Key(dest []byte, encodedKey string) (err error) {
	if len(encodedKey) != encodedKeyLen {
		err = principalKeyError("encoded principal key has wrong length")
		return
	}

	n, err := base64.RawURLEncoding.Decode(dest, []byte(encodedKey))
	if err != nil {
		err = principalKeyError("base64url encoding of principal key is invalid")
		return
	}

	if n != len(dest) {
		err = principalKeyError("decoded principal key has wrong length")
		return
	}

	return
}

func (pri *Key) PrincipalID() *ID {
	return &pri.id
}

func (pri *Key) PublicKey() ed25519.PublicKey {
	return ed25519.PublicKey(pri.id.key[:])
}

type principalKeyError string

func (s principalKeyError) Error() string       { return string(s) }
func (s principalKeyError) PublicError() string { return string(s) }
func (s principalKeyError) Unauthorized() bool  { return true }
