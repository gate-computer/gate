// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webserver

import (
	"io"
	"net/http"
	"net/url"
	"strings"

	"gate.computer/gate/server"
	"gate.computer/gate/server/api"
	"gate.computer/gate/server/event"
	"gate.computer/gate/web"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	. "import.name/type/context"
)

func mustValidateModuleKey(w http.ResponseWriter, r *http.Request, s *webserver, key string) {
	if err := server.ValidateModuleSHA256Form(key); err != nil {
		respondPathInvalid(w, r, s, err)
		panic(responded)
	}
}

func mustValidateInstanceIDInPath(w http.ResponseWriter, r *http.Request, s *webserver, id string) {
	if err := server.ValidateInstanceUUIDForm(id); err != nil {
		respondPathInvalid(w, r, s, err)
		panic(responded)
	}
}

func mustValidateInstanceIDInParam(w http.ResponseWriter, r *http.Request, s *webserver, id string) {
	if err := server.ValidateInstanceUUIDForm(id); err != nil {
		respondInvalidInstanceParam(w, r, s, id, err)
		panic(responded)
	}
}

func acceptsText(r *http.Request) bool {
	headers := r.Header[web.HeaderAccept]
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

func acceptsJSON(r *http.Request) bool {
	headers := r.Header[web.HeaderAccept]
	if len(headers) == 0 {
		return true
	}

	for _, header := range headers {
		for _, field := range strings.Split(header, ",") {
			tokens := strings.SplitN(field, ";", 2)
			mediaType := strings.TrimSpace(tokens[0])

			switch mediaType {
			case "*/*", "application/json", "application/*":
				return true
			}
		}
	}

	return false
}

func acceptsTrailers(r *http.Request) bool {
	for _, header := range r.Header[web.HeaderTE] {
		for _, field := range strings.Split(strings.ToLower(header), ",") {
			if strings.TrimSpace(field) == web.TETrailers {
				return true
			}
		}
	}

	return false
}

func mustBeAllowedOrigin(w http.ResponseWriter, r *http.Request, s *webserver, header string) {
origins:
	for _, origin := range strings.Fields(header) {
		for _, allow := range s.Origins {
			if allow == origin {
				continue origins
			}
		}

		w.WriteHeader(http.StatusForbidden)
		reportRequestError(r.Context(), s, event.FailClientDenied, "", "", "", "", nil)
		panic(responded)
	}
}

func mustAcceptJSON(w http.ResponseWriter, r *http.Request, s *webserver) {
	mustAcceptApplication(w, r, s, "application/json")
}

func mustAcceptWebAssembly(w http.ResponseWriter, r *http.Request, s *webserver) {
	mustAcceptApplication(w, r, s, "application/wasm")
}

func mustAcceptApplication(w http.ResponseWriter, r *http.Request, s *webserver, requiredType string) {
	headers := r.Header[web.HeaderAccept]
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
	panic(responded)
}

func mustNotHaveQuery(w http.ResponseWriter, r *http.Request, s *webserver) {
	if r.URL.RawQuery != "" {
		respondExcessQueryParams(w, r, s)
		panic(responded)
	}
}

func mustParseQuery(w http.ResponseWriter, r *http.Request, s *webserver) url.Values {
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		respondQueryError(w, r, s, err)
		panic(responded)
	}

	return query
}

func mustParseOptionalQuery(w http.ResponseWriter, r *http.Request, s *webserver) (query url.Values) {
	if r.URL.RawQuery != "" {
		query = mustParseQuery(w, r, s)
	}
	return
}

func popOptionalParams(query url.Values, key string) []string {
	values := query[key]
	delete(query, key)
	return values
}

func popLastParam(w http.ResponseWriter, r *http.Request, s *webserver, query url.Values, key string) string {
	switch values := query[key]; len(values) {
	case 1:
		delete(query, key)
		return values[0]

	case 0:
		if r.URL.RawQuery == "" {
			switch r.Method {
			case "GET", "HEAD":
				w.WriteHeader(http.StatusOK)
				panic(responded)
			}
		}

		respondMissingQueryParam(w, r, s, key)
		panic(responded)

	default:
		respondDuplicateQueryParam(w, r, s)
		panic(responded)
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
		panic(responded)
	}
}

func popOptionalLastLogParam(w http.ResponseWriter, r *http.Request, s *webserver, query url.Values) *api.InvokeOptions {
	switch value := popOptionalLastParam(w, r, s, query, web.ParamLog); value {
	case "":
		return nil

	case "*":
		return &api.InvokeOptions{
			DebugLog: value,
		}

	default:
		respondUnsupportedLog(w, r, s, value)
		panic(responded)
	}
}

func popOptionalActionParam(w http.ResponseWriter, r *http.Request, s *webserver, query url.Values, key string) bool {
	values := query[web.ParamAction]
	for i, s := range values {
		if s == key {
			if len(values) > 1 {
				query[web.ParamAction] = append(values[:i], values[i+1:]...)
			} else {
				delete(query, web.ParamAction)
			}
			return true
		}
	}
	return false
}

func mustPopOptionalLastFunctionParam(w http.ResponseWriter, r *http.Request, s *webserver, query url.Values) string {
	value := popOptionalLastParam(w, r, s, query, web.ParamFunction)
	if value != "" && !web.FunctionRegexp.MatchString(value) {
		respondInvalidFunction(w, r, s, value)
		panic(responded)
	}
	return value
}

func mustNotHaveParams(w http.ResponseWriter, r *http.Request, s *webserver, query url.Values) {
	if len(query) > 0 {
		respondExcessQueryParams(w, r, s)
		panic(responded)
	}
}

func getContentType(w http.ResponseWriter, r *http.Request, s *webserver) (string, bool) {
	switch values := r.Header[web.HeaderContentType]; len(values) {
	case 1:
		tokens := strings.SplitN(values[0], ";", 2)
		return strings.TrimSpace(tokens[0]), true

	case 0:
		return "", false

	default:
		respondDuplicateHeader(w, r, s, web.HeaderContentType)
		panic(responded)
	}
}

func mustHaveContentType(w http.ResponseWriter, r *http.Request, s *webserver, contentType string) {
	if value, found := getContentType(w, r, s); found {
		if value != contentType {
			respondUnsupportedMediaType(w, r, s)
			panic(responded)
		}
	} else {
		respondUnsupportedMediaType(w, r, s)
		panic(responded)
	}
}

func mustHaveContentLength(w http.ResponseWriter, r *http.Request, s *webserver) {
	if r.ContentLength < 0 {
		respondLengthRequired(w, r, s)
		panic(responded)
	}
}

func mustNotHaveContentType(w http.ResponseWriter, r *http.Request, s *webserver) {
	if _, found := r.Header[web.HeaderContentType]; found {
		respondUnsupportedMediaType(w, r, s)
		panic(responded)
	}
}

func mustNotHaveContent(w http.ResponseWriter, r *http.Request, s *webserver) {
	if r.ContentLength != 0 {
		respondContentNotEmpty(w, r, s)
		panic(responded)
	}
}

func mustParseAuthorizationHeader(ctx Context, wr *requestResponseWriter, s *webserver, require bool) Context {
	switch values := wr.request.Header[web.HeaderAuthorization]; len(values) {
	case 1:
		return mustParseAuthorization(ctx, wr, s, values[0], true)

	case 0:
		if !require {
			return ctx
		}

		respondUnauthorized(ctx, wr, s)
		panic(responded)

	default:
		// TODO: RFC 6750 says that this should be Bad Request
		respondUnauthorizedErrorDesc(ctx, wr, s, "invalid_request", "multiple Authorization headers", event.FailAuthInvalid, nil)
		panic(responded)
	}
}

func decodeProtoJSON(r io.Reader, m proto.Message) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	return protojson.Unmarshal(b, m)
}
