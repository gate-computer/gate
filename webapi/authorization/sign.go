// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package authorization

import (
	"encoding/base64"
	"encoding/json"

	"github.com/tsavola/gate/webapi"
	"golang.org/x/crypto/ed25519"
)

// BearerEd25519 creates a signed JWT token (JWS).  TokenHeader must have been
// encoded beforehand.
func BearerEd25519(privateKey ed25519.PrivateKey, encodedTokenHeader []byte, claims *webapi.Claims) (string, error) {
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	const authType = webapi.AuthorizationTypeBearer
	var enc = base64.RawURLEncoding
	var sigLen = enc.EncodedLen(ed25519.SignatureSize)
	var claimsLen = enc.EncodedLen(len(claimsJSON))

	b := make([]byte, 0, len(authType)+1+len(encodedTokenHeader)+1+claimsLen+1+sigLen)
	b = append(b, (authType + " ")...)
	b = append(b, encodedTokenHeader...)
	b = append(b, '.')
	claimsOff := len(b)
	b = b[:claimsOff+claimsLen]
	enc.Encode(b[claimsOff:], claimsJSON)
	sig := ed25519.Sign(privateKey, b[len(authType)+1:])
	b = append(b, '.')
	sigOff := len(b)
	b = b[:cap(b)]
	enc.Encode(b[sigOff:], sig)

	return string(b), nil
}
