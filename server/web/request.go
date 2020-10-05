// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package web

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"gate.computer/gate/server/event"
	"gate.computer/gate/webapi"
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

func popLastParam(w http.ResponseWriter, r *http.Request, s *webserver, query url.Values, key string) string {
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

func popOptionalLastParam(w http.ResponseWriter, r *http.Request, s *webserver, query url.Values, key string) string {
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

func popOptionalLastDebugParam(w http.ResponseWriter, r *http.Request, s *webserver, query url.Values) bool {
	return popOptionalLastParam(w, r, s, query, webapi.ParamDebug) == "true"
}

func popOptionalActionParam(w http.ResponseWriter, r *http.Request, s *webserver, query url.Values, key string) bool {
	values := query[webapi.ParamAction]
	for i, s := range values {
		if s == key {
			if len(values) > 1 {
				query[webapi.ParamAction] = append(values[:i], values[i+1:]...)
			} else {
				delete(query, webapi.ParamAction)
			}
			return true
		}
	}
	return false
}

func mustPopOptionalLastFunctionParam(w http.ResponseWriter, r *http.Request, s *webserver, query url.Values) (value string) {
	value = popOptionalLastParam(w, r, s, query, webapi.ParamFunction)
	if value != "" && !webapi.FunctionRegexp.MatchString(value) {
		respondInvalidFunction(w, r, s, value)
		panic(nil)
	}
	return
}

func mustNotHaveParams(w http.ResponseWriter, r *http.Request, s *webserver, query url.Values) {
	if len(query) > 0 {
		respondExcessQueryParams(w, r, s)
		panic(nil)
	}
}

func mustHaveContentType(w http.ResponseWriter, r *http.Request, s *webserver, contentType string) {
	switch values := r.Header[webapi.HeaderContentType]; len(values) {
	case 1:
		tokens := strings.SplitN(values[0], ";", 2)
		if strings.TrimSpace(tokens[0]) != contentType {
			respondUnsupportedMediaType(w, r, s)
			panic(nil)
		}

	case 0:
		respondUnsupportedMediaType(w, r, s)
		panic(nil)

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
	if _, found := r.Header[webapi.HeaderContentType]; found {
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

func mustParseAuthorizationHeader(ctx context.Context, wr *requestResponseWriter, s *webserver, require bool) context.Context {
	switch values := wr.request.Header[webapi.HeaderAuthorization]; len(values) {
	case 1:
		return mustParseAuthorization(ctx, wr, s, values[0], true)

	case 0:
		if !require {
			return ctx
		}

		respondUnauthorized(ctx, wr, s)
		panic(nil)

	default:
		// TODO: RFC 6750 says that this should be Bad Request
		respondUnauthorizedErrorDesc(ctx, wr, s, "invalid_request", "multiple Authorization headers", event.FailAuthInvalid, nil)
		panic(nil)
	}
}
