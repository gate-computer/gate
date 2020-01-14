// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webserver

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/tsavola/gate/internal/error/public"
	inprincipal "github.com/tsavola/gate/internal/principal"
	"github.com/tsavola/gate/principal"
	"github.com/tsavola/gate/server"
	"github.com/tsavola/gate/server/event"
	"github.com/tsavola/gate/webapi"
)

func mustParseAuthorization(ctx context.Context, ew errorWriter, s *webserver, str string, require bool) context.Context {
	if str == "" && !require {
		return nil
	}

	token := mustParseBearerToken(ctx, ew, s, str)
	return mustParseJWT(ctx, ew, s, []byte(token))
}

func mustParseBearerToken(ctx context.Context, ew errorWriter, s *webserver, str string) string {
	const bearer = webapi.AuthorizationTypeBearer

	str = strings.Trim(str, " ")
	i := strings.IndexByte(str, ' ')
	if i == len(bearer) && strings.EqualFold(str[:i], bearer) {
		return strings.TrimLeft(str[i+1:], " ")
	}

	// TODO: RFC 6750 says that this should be Bad Request
	respondUnauthorizedError(ctx, ew, s, "invalid_request")
	panic(nil)
}

func mustParseJWT(ctx context.Context, ew errorWriter, s *webserver, token []byte) context.Context {
	parts := mustSplitJWS(ctx, ew, s, token)
	signedData := token[:len(parts[0])+1+len(parts[1])]

	var (
		lenHeader  = base64.RawURLEncoding.DecodedLen(len(parts[0]))
		lenPayload = base64.RawURLEncoding.DecodedLen(len(parts[1]))
		lenSig     = base64.RawURLEncoding.DecodedLen(len(parts[2]))
	)

	buf := make([]byte, lenHeader+lenPayload+lenSig)

	var (
		bufHeader  = buf[:lenHeader]
		bufPayload = buf[lenHeader : lenHeader+lenPayload]
		bufSig     = buf[lenHeader+lenPayload:]
	)

	mustDecodeJWTComponent(ctx, ew, s, bufHeader, parts[0])
	mustDecodeJWTComponent(ctx, ew, s, bufPayload, parts[1])
	mustDecodeJWTComponent(ctx, ew, s, bufSig, parts[2])

	// Parse principal information first so that it can be used in logging.
	header := mustUnmarshalJWTHeader(ctx, ew, s, bufHeader)
	pri := mustParseJWK(ctx, ew, s, header.JWK)

	// Check expiration and audience before signature, because they are not
	// secrets.  Claims are still unauthenticated!
	claims := mustUnmarshalJWTPayload(ctx, ew, s, bufPayload)
	mustVerifyExpiration(ctx, ew, s, claims.Exp)
	mustVerifyAudience(ctx, ew, s, claims.Aud)

	// Check signature.
	mustVerifySignature(ctx, ew, s, pri, header.Alg, signedData, bufSig)

	// Check nonce after signature verification so as to not publicize
	// information about its validity.
	mustVerifyNonce(ctx, ew, s, pri, claims.Nonce, claims.Exp)

	ctx = principal.ContextWithID(ctx, pri.PrincipalID())
	ctx = server.ContextWithScope(ctx, mustValidateScope(ctx, ew, s, claims.Scope))
	return ctx
}

func mustSplitJWS(ctx context.Context, ew errorWriter, s *webserver, token []byte) [][]byte {
	if parts := bytes.SplitN(token, []byte{'.'}, 3); len(parts) == 3 {
		return parts
	}

	respondUnauthorizedError(ctx, ew, s, "invalid_token")
	panic(nil)
}

func mustDecodeJWTComponent(ctx context.Context, ew errorWriter, s *webserver, dest, src []byte) {
	n, err := base64.RawURLEncoding.Decode(dest, src)
	if err == nil && n == len(dest) {
		return
	}

	respondUnauthorizedError(ctx, ew, s, "invalid_token")
	panic(nil)
}

func mustUnmarshalJWTHeader(ctx context.Context, ew errorWriter, s *webserver, serialized []byte,
) (header webapi.TokenHeader) {
	err := json.Unmarshal(serialized, &header)
	if err == nil {
		if header.JWK != nil {
			return
		}
	}

	respondUnauthorizedError(ctx, ew, s, "invalid_token")
	panic(nil)
}

func mustUnmarshalJWTPayload(ctx context.Context, ew errorWriter, s *webserver, serialized []byte,
) (claims webapi.Claims) {
	err := json.Unmarshal(serialized, &claims)
	if err == nil {
		return
	}

	respondUnauthorizedError(ctx, ew, s, "invalid_token")
	panic(nil)
}

func mustParseJWK(ctx context.Context, ew errorWriter, s *webserver, jwk *webapi.PublicKey,
) (pri *inprincipal.Key) {
	var err error

	if jwk.Kty == webapi.KeyTypeOctetKeyPair && jwk.Crv == webapi.KeyCurveEd25519 {
		pri, err = inprincipal.ParseEd25519Key(jwk.X)
		if err == nil {
			return pri
		}

		errorDesc := public.Error(err, "principal key error")
		respondUnauthorizedErrorDesc(ctx, ew, s, "invalid_token", errorDesc, event.FailPrincipalKeyError, err)
		panic(nil)
	}

	respondUnauthorizedError(ctx, ew, s, "invalid_token")
	panic(nil)
}
