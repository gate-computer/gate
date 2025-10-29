// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package web

import (
	"bytes"
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	. "import.name/testing/mustr"
)

func TestBearerEd25519(t *testing.T) {
	pub, pri, err := ed25519.GenerateKey(bytes.NewReader(make([]byte, 1000)))
	require.NoError(t, err)

	header := TokenHeaderEdDSA(PublicKeyEd25519(pub))
	t.Logf("JWK: %#v", *header.JWK)

	t.Log("Authorization:", Must(t, R(AuthorizationBearerEd25519(pri, header.MustEncode(), &AuthorizationClaims{
		Exp: time.Now().Unix() + 300,
		Aud: []string{"test"},
	}))))
}

func TestBearerLocal(t *testing.T) {
	t.Log("Authorization:", Must(t, R(AuthorizationBearerLocal(&AuthorizationClaims{
		Aud: []string{"test"},
	}))))
}
