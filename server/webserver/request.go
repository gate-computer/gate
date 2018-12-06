// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webserver

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/tsavola/gate/server"
	"github.com/tsavola/gate/server/event"
	"github.com/tsavola/gate/webapi"
)

func acceptsText(r *http.Request) bool {
	headers := r.Header["Accept"]
	if len(headers) == 0 {
		return true
	}

	for _, header := range headers {
		for _, field := range strings.Split(header, ",") {
			tokens := strings.SplitN(field, ";", 2)
			mediaType := strings.TrimSpace(tokens[0])

			switch mediaType {
			case "*/*", "text/plain", "text/*":
				return true
			}
		}
	}

	return false
}

func mustAcceptJSON(w http.ResponseWriter, r *http.Request, s *webserver) {
	mustAcceptApplication(w, r, s, "application/json")
}

func mustAcceptWebAssembly(w http.ResponseWriter, r *http.Request, s *webserver) {
	mustAcceptApplication(w, r, s, "application/wasm")
}

func mustAcceptApplication(w http.ResponseWriter, r *http.Request, s *webserver, requiredType string) {
	headers := r.Header["Accept"]
	if len(headers) == 0 {
		return
	}

	for _, header := range headers {
		for _, field := range strings.Split(header, ",") {
			tokens := strings.SplitN(field, ";", 2)
			mediaType := strings.TrimSpace(tokens[0])

			switch mediaType {
			case requiredType, "*/*", "application/*":
				return
			}
		}
	}

	respondNotAcceptable(w, r, s)
	panic(nil)
}

func mustNotHaveQuery(w http.ResponseWriter, r *http.Request, s *webserver) {
	if r.URL.RawQuery != "" {
		respondExcessQueryParams(w, r, s)
		panic(nil)
	}
}

func mustParseQuery(w http.ResponseWriter, r *http.Request, s *webserver) url.Values {
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		respondQueryError(w, r, s, err)
		panic(nil)
	}

	return query
}

func mustParseOptionalQuery(w http.ResponseWriter, r *http.Request, s *webserver) (query url.Values) {
	if r.URL.RawQuery != "" {
		query = mustParseQuery(w, r, s)
	}
	return
}

func mustPopParam(w http.ResponseWriter, r *http.Request, s *webserver, query url.Values, key string) string {
	switch values := query[key]; len(values) {
	case 1:
		delete(query, key)
		return values[0]

	case 0:
		respondMissingQueryParam(w, r, s, key)
		panic(nil)

	default:
		respondDuplicateQueryParam(w, r, s)
		panic(nil)
	}
}

func mustPopOptionalParam(w http.ResponseWriter, r *http.Request, s *webserver, query url.Values, key string) string {
	switch values := query[key]; len(values) {
	case 1:
		delete(query, key)
		return values[0]

	case 0:
		return ""

	default:
		respondDuplicateQueryParam(w, r, s)
		panic(nil)
	}
}

func mustNotHaveParams(w http.ResponseWriter, r *http.Request, s *webserver, query url.Values) {
	if len(query) > 0 {
		respondExcessQueryParams(w, r, s)
		panic(nil)
	}
}

func mustParseContentType(w http.ResponseWriter, r *http.Request, s *webserver) string {
	switch values := r.Header[webapi.HeaderContentType]; len(values) {
	case 1:
		tokens := strings.SplitN(values[0], ";", 2)
		return strings.TrimSpace(tokens[0])

	case 0:
		return ""

	default:
		respondDuplicateHeader(w, r, s, webapi.HeaderContentType)
		panic(nil)
	}
}

func mustHaveContentLength(w http.ResponseWriter, r *http.Request, s *webserver) {
	if r.ContentLength < 0 {
		respondLengthRequired(w, r, s)
		panic(nil)
	}
}

func mustNotHaveContentType(w http.ResponseWriter, r *http.Request, s *webserver) {
	if mustParseContentType(w, r, s) != "" {
		respondUnsupportedMediaType(w, r, s)
		panic(nil)
	}
}

func mustNotHaveContent(w http.ResponseWriter, r *http.Request, s *webserver) {
	if r.ContentLength != 0 {
		respondContentNotEmpty(w, r, s)
		panic(nil)
	}
}

func mustParseAuthorizationHeader(ctx context.Context, wr *requestResponseWriter, s *webserver) (pri *server.PrincipalKey) {
	pri = mustParseOptionalAuthorizationHeader(ctx, wr, s)
	if pri == nil {
		respondUnauthorized(ctx, wr, s, nil)
		panic(nil)
	}
	return
}

func mustParseOptionalAuthorizationHeader(ctx context.Context, wr *requestResponseWriter, s *webserver) *server.PrincipalKey {
	switch values := wr.request.Header[webapi.HeaderAuthorization]; len(values) {
	case 1:
		return mustParseAuthorization(ctx, wr, s, values[0])

	case 0:
		return nil

	default:
		// TODO: RFC 6750 says that this should be Bad Request
		respondUnauthorizedErrorDesc(ctx, wr, s, nil, "invalid_request", "multiple Authorization headers", event.FailRequest_AuthInvalid, nil)
		panic(nil)
	}
}
