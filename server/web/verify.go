// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package web

import (
	"context"
	"strings"
	"time"

	"gate.computer/gate/internal/principal"
	"gate.computer/gate/server/event"
	"gate.computer/gate/server/web/api"
	"golang.org/x/crypto/ed25519"
)

const maxExpireMargin = 15 * 60 // Seconds
const maxScopeLength = 10

func mustVerifyExpiration(ctx context.Context, ew errorWriter, s *webserver, expires int64) {
	switch margin := expires - time.Now().Unix(); {
	case margin < 0:
		respondUnauthorizedErrorDesc(ctx, ew, s, "invalid_token", "token has expired", event.FailAuthExpired, nil)
		panic(nil)

	case margin > maxExpireMargin:
		respondUnauthorizedErrorDesc(ctx, ew, s, "invalid_token", "token expiration is too far in the future", event.FailAuthInvalid, nil)
		panic(nil)
	}
}

func mustVerifyAudience(ctx context.Context, ew errorWriter, s *webserver, audience []string) {
	if len(audience) == 0 {
		return
	}

	for _, a := range audience {
		if a == s.identity {
			return
		}
	}

	respondUnauthorizedError(ctx, ew, s, "invalid_token")
	panic(nil)
}

func mustVerifySignature(ctx context.Context, ew errorWriter, s *webserver, pri *principal.Key, alg string, signedData, signature []byte) {
	switch alg {
	case api.SignAlgEdDSA:
		if ed25519.Verify(pri.PublicKey(), signedData, signature) {
			return
		}

	case api.SignAlgNone:
		if len(signature) == 0 {
			if pri != nil {
				panic(pri)
			}
			if s.localAuthorization {
				return
			}
			panic("unsigned token without local authorization")
		}
	}

	respondUnauthorizedError(ctx, ew, s, "invalid_token")
	panic(nil)
}

func mustVerifyNonce(ctx context.Context, ew errorWriter, s *webserver, pri *principal.Key, nonce string, expires int64) {
	if nonce == "" {
		return
	}

	if s.NonceStorage == nil {
		respondUnauthorizedErrorDesc(ctx, ew, s, "invalid_token", "nonce not supported", event.FailAuthInvalid, nil)
		panic(nil)
	}

	if err := s.NonceStorage.CheckNonce(ctx, pri.PublicKey(), nonce, time.Unix(expires, 0)); err != nil {
		respondUnauthorizedErrorDesc(ctx, ew, s, "invalid_token", "token has already been used", event.FailAuthReused, err)
		panic(nil)
	}
}

func mustValidateScope(ctx context.Context, ew errorWriter, s *webserver, scope string) []string {
	array := strings.SplitN(scope, " ", maxScopeLength)
	if len(array) == maxScopeLength && strings.Index(array[maxScopeLength-1], " ") >= 0 {
		respondUnauthorizedErrorDesc(ctx, ew, s, "invalid_token", "scope has too many tokens", event.FailScopeTooLarge, nil)
		panic(nil)
	}

	return array
}
