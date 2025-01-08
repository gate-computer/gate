// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package web

import (
	"bytes"
	"crypto/ed25519"
	"testing"
	"time"
)

func TestBearerEd25519(t *testing.T) {
	pub, pri, err := ed25519.GenerateKey(bytes.NewReader(make([]byte, 1000)))
	if err != nil {
		t.Fatal(err)
	}

	header := TokenHeaderEdDSA(PublicKeyEd25519(pub))

	t.Logf("JWK: %#v", *header.JWK)

	claims := &AuthorizationClaims{
		Exp: time.Now().Unix() + 300,
		Aud: []string{"test"},
	}

	authorization, err := AuthorizationBearerEd25519(pri, header.MustEncode(), claims)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Authorization: %s", authorization)
}

func TestBearerLocal(t *testing.T) {
	claims := &AuthorizationClaims{
		Aud: []string{"test"},
	}

	authorization, err := AuthorizationBearerLocal(claims)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Authorization: %s", authorization)
}
