// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webserver

import (
	"crypto/ed25519"
	"slices"
	"strings"
	"time"

	"gate.computer/gate/server/event"
	"gate.computer/gate/web"
	"gate.computer/internal/principal"

	. "import.name/type/context"
)

const (
	maxExpireMargin = 15 * 60 // Seconds
	maxScopeLength  = 10
)

func mustVerifyExpiration(ctx Context, ew errorWriter, s *webserver, expires int64) {
	if expires == 0 && s.localAuthorization {
		return
	}

	switch margin := expires - time.Now().Unix(); {
	case margin < 0:
		respondUnauthorizedErrorDesc(ctx, ew, s, "invalid_token", "token has expired", event.FailAuthExpired, nil)
		panic(responded)

	case margin > maxExpireMargin:
		respondUnauthorizedErrorDesc(ctx, ew, s, "invalid_token", "token expiration is too far in the future", event.FailAuthInvalid, nil)
		panic(responded)
	}
}

func mustVerifyAudience(ctx Context, ew errorWriter, s *webserver, audience []string) {
	if len(audience) == 0 {
		return
	}

	if slices.Contains(audience, s.identity) {
		return
	}

	respondUnauthorizedError(ctx, ew, s, "invalid_token")
	panic(responded)
}

func mustVerifySignature(ctx Context, ew errorWriter, s *webserver, pri *principal.Key, alg string, signedData, signature []byte) {
	switch alg {
	case web.SignAlgEdDSA:
		if ed25519.Verify(pri.PublicKey(), signedData, signature) {
			return
		}

	case web.SignAlgNone:
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
	panic(responded)
}

func mustVerifyNonce(ctx Context, ew errorWriter, s *webserver, pri *principal.Key, nonce string, expires int64) {
	if nonce == "" {
		return
	}

	if s.NonceChecker == nil {
		respondUnauthorizedErrorDesc(ctx, ew, s, "invalid_token", "nonce not supported", event.FailAuthInvalid, nil)
		panic(responded)
	}

	if err := s.NonceChecker.CheckNonce(ctx, pri.PublicKey(), nonce, time.Unix(expires, 0)); err != nil {
		respondUnauthorizedErrorDesc(ctx, ew, s, "invalid_token", "token has already been used", event.FailAuthReused, err)
		panic(responded)
	}
}

func mustValidateScope(ctx Context, ew errorWriter, s *webserver, scope string) []string {
	array := strings.SplitN(scope, " ", maxScopeLength)
	if len(array) == maxScopeLength && strings.Contains(array[maxScopeLength-1], " ") {
		respondUnauthorizedErrorDesc(ctx, ew, s, "invalid_token", "scope has too many tokens", event.FailScopeTooLarge, nil)
		panic(responded)
	}

	return array
}
