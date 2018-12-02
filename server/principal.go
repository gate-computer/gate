// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"encoding/base64"
)

const (
	principalKeySize = 32
	encodedKeyLen    = 43
)

type PrincipalKey struct {
	key [principalKeySize]byte

	PrincipalID string
}

func ParsePrincipalKey(principalID, encodedKey string) (pri *PrincipalKey, err error) {
	pri = &PrincipalKey{
		PrincipalID: principalID,
	}

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

	return
}

func (pri *PrincipalKey) KeyBytes() []byte {
	return pri.key[:]
}

// KeyPtr returns a pointer to byte array of the given size if the key size
// matches it.  Otherwise it returns an unspecified value that cannot be
// converted to a pointer to a byte array of the given size.
func (pri *PrincipalKey) KeyPtr(size int) interface{} {
	return &pri.key
}

type principalKeyError string

func (s principalKeyError) Error() string       { return string(s) }
func (s principalKeyError) PublicError() string { return string(s) }
func (s principalKeyError) Unauthorized()       {}
