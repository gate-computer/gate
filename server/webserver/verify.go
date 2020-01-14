// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webserver

import (
	"context"
	"time"

	"github.com/tsavola/gate/internal/principal"
	"github.com/tsavola/gate/server/event"
	"github.com/tsavola/gate/webapi"
	"golang.org/x/crypto/ed25519"
)

const maxExpireMargin = 15 * 60 // Seconds

func mustVerifyExpiration(ctx context.Context, ew errorWriter, s *webserver, pri *principal.ID, expires int64) {
	switch margin := expires - time.Now().Unix(); {
	case margin < 0:
		respondUnauthorizedErrorDesc(ctx, ew, s, pri, "invalid_token", "token has expired", event.FailAuthExpired, nil)
		panic(nil)

	case margin > maxExpireMargin:
		respondUnauthorizedErrorDesc(ctx, ew, s, pri, "invalid_token", "token expiration is too far in the future", event.FailAuthInvalid, nil)
		panic(nil)
	}
}

func mustVerifyAudience(ctx context.Context, ew errorWriter, s *webserver, pri *principal.ID, audience []string) {
	if len(audience) == 0 {
		return
	}

	for _, a := range audience {
		if a == s.identity {
			return
		}
	}

	respondUnauthorizedError(ctx, ew, s, pri, "invalid_token")
	panic(nil)
}

func mustVerifySignature(ctx context.Context, ew errorWriter, s *webserver, pri *principal.Key, algorithm string, signedData, signature []byte) {
	if algorithm == webapi.SignAlgEdDSA {
		if ed25519.Verify(pri.PublicKey(), signedData, signature) {
			return
		}
	}

	respondUnauthorizedError(ctx, ew, s, pri.PrincipalID(), "invalid_token")
	panic(nil)
}

func mustVerifyNonce(ctx context.Context, ew errorWriter, s *webserver, pri *principal.Key, nonce string, expires int64) {
	if nonce == "" {
		return
	}

	if s.NonceStorage == nil {
		respondUnauthorizedErrorDesc(ctx, ew, s, pri.PrincipalID(), "invalid_token", "nonce not supported", event.FailAuthInvalid, nil)
		panic(nil)
	}

	if err := s.NonceStorage.CheckNonce(ctx, pri.PublicKey(), nonce, time.Unix(expires, 0)); err != nil {
		respondUnauthorizedErrorDesc(ctx, ew, s, pri.PrincipalID(), "invalid_token", "token has already been used", event.FailAuthReused, err)
		panic(nil)
	}
}
