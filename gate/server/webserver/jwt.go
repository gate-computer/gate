// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webserver

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"

	"gate.computer/gate/scope"
	"gate.computer/gate/server/api"
	"gate.computer/gate/server/event"
	"gate.computer/gate/web"
	"gate.computer/internal/principal"

	. "import.name/type/context"
)

func mustParseAuthorization(ctx Context, ew errorWriter, s *webserver, str string, require bool) Context {
	if str == "" && !require {
		return ctx
	}

	token := mustParseBearerToken(ctx, ew, s, str)
	return mustParseJWT(ctx, ew, s, []byte(token))
}

func mustParseBearerToken(ctx Context, ew errorWriter, s *webserver, str string) string {
	const bearer = web.AuthorizationTypeBearer

	str = strings.Trim(str, " ")
	i := strings.IndexByte(str, ' ')
	if i == len(bearer) && strings.EqualFold(str[:i], bearer) {
		return strings.TrimLeft(str[i+1:], " ")
	}

	// TODO: RFC 6750 says that this should be Bad Request
	respondUnauthorizedError(ctx, ew, s, "invalid_request")
	panic(responded)
}

func mustParseJWT(ctx Context, ew errorWriter, s *webserver, token []byte) Context {
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
	pri := mustParseJWTHeader(ctx, ew, s, header)

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

	switch {
	case pri != nil:
		ctx = principal.ContextWithID(ctx, pri.PrincipalID())

	case pri == nil && s.localAuthorization:
		ctx = principal.ContextWithID(ctx, principal.LocalID)

	default:
		panic("no principal key and no local authorization")
	}

	return scope.Context(ctx, mustValidateScope(ctx, ew, s, claims.Scope))
}

func mustSplitJWS(ctx Context, ew errorWriter, s *webserver, token []byte) [][]byte {
	if parts := bytes.SplitN(token, []byte{'.'}, 3); len(parts) == 3 {
		return parts
	}

	respondUnauthorizedError(ctx, ew, s, "invalid_token")
	panic(responded)
}

func mustDecodeJWTComponent(ctx Context, ew errorWriter, s *webserver, dest, src []byte) {
	n, err := base64.RawURLEncoding.Decode(dest, src)
	if err == nil && n == len(dest) {
		return
	}

	respondUnauthorizedError(ctx, ew, s, "invalid_token")
	panic(responded)
}

func mustUnmarshalJWTHeader(ctx Context, ew errorWriter, s *webserver, serialized []byte) web.TokenHeader {
	var header web.TokenHeader
	if err := json.Unmarshal(serialized, &header); err == nil {
		return header
	}

	respondUnauthorizedError(ctx, ew, s, "invalid_token")
	panic(responded)
}

func mustUnmarshalJWTPayload(ctx Context, ew errorWriter, s *webserver, serialized []byte) web.Claims {
	var claims web.Claims
	if err := json.Unmarshal(serialized, &claims); err == nil {
		return claims
	}

	respondUnauthorizedError(ctx, ew, s, "invalid_token")
	panic(responded)
}

func mustParseJWTHeader(ctx Context, ew errorWriter, s *webserver, header web.TokenHeader) *principal.Key {
	switch header.Alg {
	case web.SignAlgEdDSA:
		k := header.JWK
		if k.Kty == web.KeyTypeOctetKeyPair && k.Crv == web.KeyCurveEd25519 {
			pri, err := principal.ParseEd25519Key(k.X)
			if err == nil {
				return pri
			}

			errorDesc := api.PublicErrorString(err, "principal key error")
			respondUnauthorizedErrorDesc(ctx, ew, s, "invalid_token", errorDesc, event.FailPrincipalKeyError, err)
			panic(responded)
		}

	case web.SignAlgNone:
		if s.localAuthorization {
			return nil
		}
	}

	respondUnauthorizedError(ctx, ew, s, "invalid_token")
	panic(responded)
}
