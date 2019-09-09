// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/tsavola/gate/internal/error/badprogram"
	"github.com/tsavola/gate/internal/error/badrequest"
	"github.com/tsavola/gate/internal/error/notfound"
	"github.com/tsavola/gate/internal/error/public"
	"github.com/tsavola/gate/internal/error/resourcelimit"
	"github.com/tsavola/gate/principal"
	"github.com/tsavola/gate/server"
	"github.com/tsavola/gate/server/event"
	"github.com/tsavola/gate/server/internal/error/failrequest"
	"github.com/tsavola/gate/server/internal/error/resourcenotfound"
	"github.com/tsavola/gate/webapi"
)

const (
	accessControlMaxAge = "86400"
	cacheControlStatic  = "public, max-age=3600"
	contentTypeText     = "text/plain; charset=utf-8"
	contentTypeJSON     = "application/json; charset=utf-8"
)

var (
	errContentDecode        = errors.New("content decode error")
	errContentNotEmpty      = errors.New("request content not empty")
	errDuplicateQueryParam  = errors.New("duplicate query parameter")
	errEncodingNotSupported = errors.New("unsupported content encoding")
	errLengthRequired       = errors.New("length required")
	errMethodNotAllowed     = errors.New("method not allowed")
	errNotAcceptable        = errors.New("not acceptable")
	errUnsupportedMediaType = errors.New("unsupported content type")
)

func join(fields ...string) string {
	return strings.Join(fields, ", ")
}

func mustMarshalJSON(x interface{}) []byte {
	content, err := json.MarshalIndent(x, "", "\t")
	if err != nil {
		panic(err)
	}
	return append(content, '\n')
}

func setAccessControl(w http.ResponseWriter, r *http.Request, methods string) (originSet bool) {
	_, originSet = r.Header["Origin"]
	if originSet {
		w.Header().Set("Access-Control-Allow-Methods", methods)
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Max-Age", accessControlMaxAge)
	}
	return
}

func setAccessControlAllowHeaders(w http.ResponseWriter, r *http.Request, methods, headers string,
) (originSet bool) {
	originSet = setAccessControl(w, r, methods)
	if originSet {
		w.Header().Set("Access-Control-Allow-Headers", headers)
	}
	return
}

func setAccessControlAllowExposeHeaders(w http.ResponseWriter, r *http.Request, methods string, allowHeaders string, exposeHeaders string) {
	if setAccessControlAllowHeaders(w, r, methods, allowHeaders) {
		w.Header().Set("Access-Control-Expose-Headers", exposeHeaders)
	}
}

func setOptions(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
}

func respond(w http.ResponseWriter, r *http.Request, status int, text string) {
	if acceptsText(r) {
		w.Header().Set(webapi.HeaderContentType, contentTypeText)
		w.WriteHeader(status)
		fmt.Fprintln(w, text)
	} else {
		w.WriteHeader(status)
	}
}

type requestResponseWriter struct {
	response http.ResponseWriter
	request  *http.Request
}

func (wr *requestResponseWriter) SetHeader(key, value string) {
	wr.response.Header().Set(key, value)
}

func (wr *requestResponseWriter) WriteError(status int, text string) {
	respond(wr.response, wr.request, status, text)
}

func respondMethodNotAllowed(w http.ResponseWriter, r *http.Request, s *webserver, allow string) {
	w.Header().Set("Allow", allow)
	respond(w, r, http.StatusMethodNotAllowed, errMethodNotAllowed.Error())
	reportProtocolError(r.Context(), s, nil, errMethodNotAllowed)
}

func respondNotAcceptable(w http.ResponseWriter, r *http.Request, s *webserver) {
	respond(w, r, http.StatusNotAcceptable, errNotAcceptable.Error())
	reportProtocolError(r.Context(), s, nil, errNotAcceptable)
}

func respondUnsupportedMediaType(w http.ResponseWriter, r *http.Request, s *webserver) {
	respond(w, r, http.StatusUnsupportedMediaType, errUnsupportedMediaType.Error())
	reportProtocolError(r.Context(), s, nil, errUnsupportedMediaType)
}

func respondLengthRequired(w http.ResponseWriter, r *http.Request, s *webserver) {
	respond(w, r, http.StatusLengthRequired, errLengthRequired.Error())
	reportProtocolError(r.Context(), s, nil, errLengthRequired)
}

func respondContentNotEmpty(w http.ResponseWriter, r *http.Request, s *webserver) {
	respond(w, r, http.StatusRequestEntityTooLarge, "request content must be empty")
	reportProtocolError(r.Context(), s, nil, errContentNotEmpty)
}

func respondPathNotFound(w http.ResponseWriter, r *http.Request, s *webserver) {
	respond(w, r, http.StatusNotFound, "not found")
	reportProtocolError(r.Context(), s, nil, fmt.Errorf("path not found: %s", r.URL.Path))
}

func respondDuplicateHeader(w http.ResponseWriter, r *http.Request, s *webserver, key string) {
	err := fmt.Errorf("duplicate header: %s", key)
	respond(w, r, http.StatusBadRequest, err.Error())
	reportProtocolError(r.Context(), s, nil, err)
}

func respondQueryError(w http.ResponseWriter, r *http.Request, s *webserver, err error) {
	respond(w, r, http.StatusBadRequest, "query string decode error")
	reportProtocolError(r.Context(), s, nil, err)
}

func respondMissingQueryParam(w http.ResponseWriter, r *http.Request, s *webserver, key string) {
	err := fmt.Errorf("missing query parameter: %s", key)
	respond(w, r, http.StatusBadRequest, err.Error())
	reportProtocolError(r.Context(), s, nil, err)
}

func respondInvalidFunction(w http.ResponseWriter, r *http.Request, s *webserver, value string) {
	err := fmt.Errorf("invalid function name: %q", value)
	respond(w, r, http.StatusBadRequest, err.Error())
	reportProtocolError(r.Context(), s, nil, err)
}

func respondDuplicateQueryParam(w http.ResponseWriter, r *http.Request, s *webserver) {
	respond(w, r, http.StatusBadRequest, errDuplicateQueryParam.Error())
	reportProtocolError(r.Context(), s, nil, errDuplicateQueryParam)
}

func respondExcessQueryParams(w http.ResponseWriter, r *http.Request, s *webserver) {
	respond(w, r, http.StatusBadRequest, "unexpected query parameters")
	reportProtocolError(r.Context(), s, nil, fmt.Errorf("unexpected query params: %s", r.URL.RawQuery))
}

func respondUnsupportedAction(w http.ResponseWriter, r *http.Request, s *webserver) {
	respond(w, r, http.StatusBadRequest, "unsupported action")
	reportProtocolError(r.Context(), s, nil, fmt.Errorf("bad action query: %s", r.URL.RawQuery))
}

func respondUnauthorized(ctx context.Context, ew errorWriter, s *webserver, pri *principal.Key) {
	ew.SetHeader("Www-Authenticate", fmt.Sprintf("%s realm=%q", webapi.AuthorizationTypeBearer, s.identity))
	ew.WriteError(http.StatusUnauthorized, "missing authentication credentials")
	reportRequestFailure(ctx, s, pri, event.FailAuthMissing)
}

func respondUnauthorizedError(ctx context.Context, ew errorWriter, s *webserver, pri *principal.Key, errorCode string) {
	ew.SetHeader("Www-Authenticate", fmt.Sprintf("%s realm=%q error=%q", webapi.AuthorizationTypeBearer, s.identity, errorCode))
	ew.WriteError(http.StatusUnauthorized, errorCode)
	reportRequestFailure(ctx, s, pri, event.FailAuthInvalid)
}

func respondUnauthorizedErrorDesc(ctx context.Context, ew errorWriter, s *webserver, pri *principal.Key, errorCode, errorDesc string, failType event.FailRequest_Type, err error) {
	ew.SetHeader("Www-Authenticate", fmt.Sprintf("%s realm=%q error=%q error_description=%q", webapi.AuthorizationTypeBearer, s.identity, errorCode, errorDesc))
	ew.WriteError(http.StatusUnauthorized, errorDesc)
	reportRequestError(ctx, s, pri, failType, "", "", "", "", err)
}

func respondContentDecodeError(ctx context.Context, ew errorWriter, s *webserver, pri *principal.Key, err error) {
	ew.WriteError(http.StatusBadRequest, errContentDecode.Error())
	reportPayloadError(ctx, s, pri, errContentDecode)
}

func respondUnsupportedEncoding(ctx context.Context, ew errorWriter, s *webserver, pri *principal.Key) {
	ew.WriteError(http.StatusBadRequest, errEncodingNotSupported.Error())
	reportPayloadError(ctx, s, pri, errEncodingNotSupported)
}

func respondServerError(ctx context.Context, ew errorWriter, s *webserver, pri *principal.Key, sourceURI, progHash, function, instID string, err error) {
	var (
		status   = http.StatusInternalServerError
		text     = "internal server error"
		internal = true
		request  = event.FailUnspecified
	)

	switch x := err.(type) {
	case server.Unauthorized:
		status = http.StatusUnauthorized
		text = "unauthorized"
		internal = false
		request = event.FailAuthDenied

		ew.SetHeader("Www-Authenticate", fmt.Sprintf("%s realm=%q", webapi.AuthorizationTypeBearer, s.identity))

	case server.Forbidden:
		status = http.StatusForbidden
		text = "forbidden"
		internal = false
		request = event.FailResourceDenied

		switch err.(type) {
		case resourcelimit.Error:
			request = event.FailResourceLimit
		}

	case server.TooManyRequests:
		status = http.StatusTooManyRequests
		text = "too many requests"
		internal = false
		request = event.FailRateLimit

		if d := x.RetryAfter(); d != 0 {
			s := x.RetryAfter() / time.Second
			if s == 0 {
				s = 1
			}
			ew.SetHeader("Retry-After", strconv.Itoa(int(s)))
		}

	case notfound.Error:
		status = http.StatusNotFound
		text = "not found"
		internal = false

		switch err.(type) {
		case resourcenotfound.ModuleError:
			text = "module not found"
			request = event.FailModuleNotFound

		case notfound.FunctionError:
			text = "function not found"
			request = event.FailFunctionNotFound

		case resourcenotfound.InstanceError:
			text = "instance not found"
			request = event.FailInstanceNotFound
		}

	case badrequest.Error:
		status = http.StatusBadRequest
		text = "bad request"
		internal = false

		switch x := err.(type) {
		case badprogram.Error:
			text = "bad module"
			request = event.FailModuleError

		case failrequest.Error:
			request = x.FailRequestType()
		}
	}

	if x, ok := err.(public.Error); ok {
		text = x.PublicError()
	}

	ew.WriteError(status, text)

	if internal {
		reportInternalError(ctx, s, pri, sourceURI, progHash, function, instID, err)
	} else {
		reportRequestError(ctx, s, pri, request, sourceURI, progHash, function, instID, err)
	}
}
