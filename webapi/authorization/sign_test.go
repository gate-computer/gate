// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package authorization

import (
	"bytes"
	"testing"
	"time"

	"github.com/tsavola/gate/webapi"
	"golang.org/x/crypto/ed25519"
)

func TestBearerEd25519(t *testing.T) {
	pub, pri, err := ed25519.GenerateKey(bytes.NewReader(make([]byte, 1000)))
	if err != nil {
		t.Fatal(err)
	}

	header := webapi.TokenHeaderEdDSA(webapi.PublicKeyEd25519(pub))

	t.Logf("JWK: %#v", *header.JWK)

	claims := &webapi.Claims{
		Exp: time.Now().Unix() + 300,
		Aud: []string{"test"},
	}

	authorization, err := BearerEd25519(pri, header.MustEncode(), claims)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Authorization: %s", authorization)
}
