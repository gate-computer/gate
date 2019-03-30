// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package principal

import (
	"context"
	"encoding/base64"
)

const (
	keySize       = 32
	encodedKeyLen = 43
)

type Key struct {
	key         [keySize]byte
	principalID ID
}

func ParseEd25519Key(encodedKey string) (pri *Key, err error) {
	pri = new(Key)

	if len(encodedKey) != encodedKeyLen {
		err = principalKeyError("encoded principal key has wrong length")
		return
	}

	n, err := base64.RawURLEncoding.Decode(pri.key[:], []byte(encodedKey))
	if err != nil {
		err = principalKeyError("base64url encoding of principal key is invalid")
		return
	}

	if n != len(pri.key) {
		err = principalKeyError("decoded principal key has wrong length")
		return
	}

	pri.principalID = makeEd25519ID(encodedKey)
	return
}

func RawKey(pri *Key) [keySize]byte {
	return pri.key
}

func KeyPrincipalID(pri *Key) ID {
	return pri.principalID
}

func ContextWithIDFrom(ctx context.Context, key *Key) context.Context {
	if key == nil {
		return ctx
	}

	return ContextWithID(ctx, key.principalID)
}

type principalKeyError string

func (s principalKeyError) Error() string       { return string(s) }
func (s principalKeyError) PublicError() string { return string(s) }
func (s principalKeyError) Unauthorized()       {}
