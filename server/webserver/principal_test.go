// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webserver

import (
	"github.com/tsavola/gate/webapi"
	"github.com/tsavola/gate/webapi/authorization"

	"golang.org/x/crypto/ed25519"
)

type testKey struct {
	private     ed25519.PrivateKey
	tokenHeader []byte
}

func newTestKey() *testKey {
	pub, pri, err := ed25519.GenerateKey(nil)
	if err != nil {
		panic(err)
	}

	return &testKey{pri, webapi.TokenHeaderEdDSA(webapi.PublicKeyEd25519(pub)).MustEncode()}
}

func (key *testKey) authorization(claims *webapi.Claims) string {
	s, err := authorization.BearerEd25519(key.private, key.tokenHeader, claims)
	if err != nil {
		panic(err)
	}
	return s
}
