// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webserver

import (
	"context"
	"time"

	"github.com/tsavola/gate/server"
	"github.com/tsavola/gate/server/event"
	"github.com/tsavola/gate/webapi"
	"golang.org/x/crypto/ed25519"
)

const maxExpireMargin = 15 * 60 // Seconds

func mustVerifyExpiration(ctx context.Context, ew errorWriter, s *webserver, pri *server.PrincipalKey, expires int64) {
	switch margin := expires - time.Now().Unix(); {
	case margin < 0:
		respondUnauthorizedErrorDesc(ctx, ew, s, pri, "invalid_token", "token has expired", event.FailRequest_AuthExpired, nil)
		panic(nil)

	case margin > maxExpireMargin:
		respondUnauthorizedErrorDesc(ctx, ew, s, pri, "invalid_token", "token expiration is too far in the future", event.FailRequest_AuthInvalid, nil)
		panic(nil)
	}
}

func mustVerifyAudience(ctx context.Context, ew errorWriter, s *webserver, pri *server.PrincipalKey, audience []string) {
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

func mustVerifySignature(ctx context.Context, ew errorWriter, s *webserver, pri *server.PrincipalKey, algorithm string, signedData, signature []byte) {
	if algorithm == webapi.SignAlgEdDSA {
		if ed25519.Verify(ed25519.PublicKey(pri.KeyBytes()), signedData, signature) {
			return
		}
	}

	respondUnauthorizedError(ctx, ew, s, pri, "invalid_token")
	panic(nil)
}

func mustVerifyNonce(ctx context.Context, ew errorWriter, s *webserver, pri *server.PrincipalKey, nonce string, expires int64) {
	if nonce == "" {
		return
	}

	if s.AccessState == nil {
		respondUnauthorizedErrorDesc(ctx, ew, s, pri, "invalid_token", "nonce not supported", event.FailRequest_AuthInvalid, err)
		panic(nil)
	}

	if err := s.AccessState.TrackNonce(ctx, pri, nonce, time.Unix(expires, 0)); err != nil {
		respondUnauthorizedErrorDesc(ctx, ew, s, pri, "invalid_token", "token has already been used", event.FailRequest_AuthReused, err)
		panic(nil)
	}
}
