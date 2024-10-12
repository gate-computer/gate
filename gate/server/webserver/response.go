// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gate.computer/gate/server/api"
	"gate.computer/gate/server/event"
	"gate.computer/gate/web"

	. "import.name/type/context"
)

const (
	accessControlMaxAge = "86400"
	cacheControlStatic  = "public, max-age=3600"
	contentTypeText     = "text/plain; charset=utf-8"
	contentTypeJSON     = "application/json; charset=utf-8"
)

var (
	errContentNotEmpty      = errors.New("request content not empty")
	errDuplicateQueryParam  = errors.New("duplicate query parameter")
	errLengthRequired       = errors.New("length required")
	errMethodNotAllowed     = errors.New("method not allowed")
	errNotAcceptable        = errors.New("not acceptable")
	errUnsupportedMediaType = errors.New("unsupported content type")
)

func join(fields ...string) string {
	return strings.Join(fields, ", ")
}

func mustMarshalJSON(x any) []byte {
	content, err := json.MarshalIndent(x, "", "\t")
	if err != nil {
		panic(err)
	}
	return append(content, '\n')
}

// setAccessControl returns true if request contained Origin header.
func setAccessControl(w http.ResponseWriter, r *http.Request, s *webserver, methods string) bool {
	_, originSet := r.Header[web.HeaderOrigin]
	if originSet {
		origin := "*"
		if !s.anyOrigin {
			origin = r.Header.Get(web.HeaderOrigin)
		}

		w.Header().Set("Access-Control-Allow-Methods", methods)
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Max-Age", accessControlMaxAge)
	}

	return originSet
}

// setAccessControlAllowHeaders returns true if request contained Origin header.
func setAccessControlAllowHeaders(w http.ResponseWriter, r *http.Request, s *webserver, methods, headers string) bool {
	originSet := setAccessControl(w, r, s, methods)
	if originSet {
		w.Header().Set("Access-Control-Allow-Headers", headers)
	}

	return originSet
}

func setAccessControlAllowExposeHeaders(w http.ResponseWriter, r *http.Request, s *webserver, methods, allowHeaders, exposeHeaders string) {
	if setAccessControlAllowHeaders(w, r, s, methods, allowHeaders) {
		w.Header().Set("Access-Control-Expose-Headers", exposeHeaders)
	}
}

func setOptions(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
}

func respond(w http.ResponseWriter, r *http.Request, status int, text string) {
	if acceptsText(r) {
		w.Header().Set(web.HeaderContentType, contentTypeText)
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
	w.Header().Set("Cache-Control", cacheControlStatic)
	respond(w, r, http.StatusMethodNotAllowed, errMethodNotAllowed.Error())
	reportProtocolError(r.Context(), s, errMethodNotAllowed)
}

func respondNotAcceptable(w http.ResponseWriter, r *http.Request, s *webserver) {
	respond(w, r, http.StatusNotAcceptable, errNotAcceptable.Error())
	reportProtocolError(r.Context(), s, errNotAcceptable)
}

func respondUnsupportedMediaType(w http.ResponseWriter, r *http.Request, s *webserver) {
	respond(w, r, http.StatusUnsupportedMediaType, errUnsupportedMediaType.Error())
	reportProtocolError(r.Context(), s, errUnsupportedMediaType)
}

func respondLengthRequired(w http.ResponseWriter, r *http.Request, s *webserver) {
	respond(w, r, http.StatusLengthRequired, errLengthRequired.Error())
	reportProtocolError(r.Context(), s, errLengthRequired)
}

func respondContentNotEmpty(w http.ResponseWriter, r *http.Request, s *webserver) {
	respond(w, r, http.StatusRequestEntityTooLarge, "request content must be empty")
	reportProtocolError(r.Context(), s, errContentNotEmpty)
}

func respondPathNotFound(w http.ResponseWriter, r *http.Request, s *webserver) {
	respond(w, r, http.StatusNotFound, "not found")
	reportProtocolError(r.Context(), s, fmt.Errorf("path not found: %s", r.URL.Path))
}

func respondPathInvalid(w http.ResponseWriter, r *http.Request, s *webserver, err error) {
	respond(w, r, http.StatusNotFound, api.PublicErrorString(err, "invalid path"))
	reportProtocolError(r.Context(), s, fmt.Errorf("path %q: %w", r.URL.Path, err))
}

func respondDuplicateHeader(w http.ResponseWriter, r *http.Request, s *webserver, key string) {
	err := fmt.Errorf("duplicate header: %s", key)
	respond(w, r, http.StatusBadRequest, err.Error())
	reportProtocolError(r.Context(), s, err)
}

func respondQueryError(w http.ResponseWriter, r *http.Request, s *webserver, err error) {
	respond(w, r, http.StatusBadRequest, "query string decode error")
	reportProtocolError(r.Context(), s, err)
}

func respondMissingQueryParam(w http.ResponseWriter, r *http.Request, s *webserver, key string) {
	err := fmt.Errorf("missing query parameter: %s", key)
	respond(w, r, http.StatusBadRequest, err.Error())
	reportProtocolError(r.Context(), s, err)
}

func respondInvalidInstanceParam(w http.ResponseWriter, r *http.Request, s *webserver, value string, err error) {
	respond(w, r, http.StatusBadRequest, api.PublicErrorString(err, "invalid instance id"))
	reportProtocolError(r.Context(), s, fmt.Errorf("instance %q: %w", value, err))
}

func respondInvalidFunction(w http.ResponseWriter, r *http.Request, s *webserver, value string) {
	err := fmt.Errorf("invalid function name: %q", value)
	respond(w, r, http.StatusBadRequest, err.Error())
	reportProtocolError(r.Context(), s, err)
}

func respondUnsupportedLog(w http.ResponseWriter, r *http.Request, s *webserver, value string) {
	err := fmt.Errorf("unsupported log argument: %q", value)
	respond(w, r, http.StatusNotImplemented, err.Error())
	reportProtocolError(r.Context(), s, err)
}

func respondDuplicateQueryParam(w http.ResponseWriter, r *http.Request, s *webserver) {
	respond(w, r, http.StatusBadRequest, errDuplicateQueryParam.Error())
	reportProtocolError(r.Context(), s, errDuplicateQueryParam)
}

func respondExcessQueryParams(w http.ResponseWriter, r *http.Request, s *webserver) {
	respond(w, r, http.StatusBadRequest, "unexpected query parameters")
	reportProtocolError(r.Context(), s, fmt.Errorf("unexpected query params: %s", r.URL.RawQuery))
}

func respondUnsupportedAction(w http.ResponseWriter, r *http.Request, s *webserver) {
	w.Header().Set("Cache-Control", cacheControlStatic)
	respond(w, r, http.StatusNotImplemented, "unsupported action")
	reportProtocolError(r.Context(), s, fmt.Errorf("bad action query: %s", r.URL.RawQuery))
}

func respondUnsupportedFeature(w http.ResponseWriter, r *http.Request, s *webserver) {
	w.Header().Set("Cache-Control", cacheControlStatic)
	respond(w, r, http.StatusNotImplemented, "unsupported feature")
	reportProtocolError(r.Context(), s, fmt.Errorf("bad action query: %s", r.URL.RawQuery))
}

func respondUnauthorized(ctx Context, ew errorWriter, s *webserver) {
	ew.SetHeader("Www-Authenticate", fmt.Sprintf("%s realm=%q", web.AuthorizationTypeBearer, s.identity))
	ew.WriteError(http.StatusUnauthorized, "missing authentication credentials")
	reportRequestFailure(ctx, s, event.FailAuthMissing)
}

func respondUnauthorizedError(ctx Context, ew errorWriter, s *webserver, errorCode string) {
	ew.SetHeader("Www-Authenticate", fmt.Sprintf("%s realm=%q error=%q", web.AuthorizationTypeBearer, s.identity, errorCode))
	ew.WriteError(http.StatusUnauthorized, errorCode)
	reportRequestFailure(ctx, s, event.FailAuthInvalid)
}

func respondUnauthorizedErrorDesc(ctx Context, ew errorWriter, s *webserver, errorCode, errorDesc string, failType event.FailType, err error) {
	ew.SetHeader("Www-Authenticate", fmt.Sprintf("%s realm=%q error=%q error_description=%q", web.AuthorizationTypeBearer, s.identity, errorCode, errorDesc))
	ew.WriteError(http.StatusUnauthorized, errorDesc)
	reportRequestError(ctx, s, failType, "", "", "", "", err)
}

func respondContentParseError(ctx Context, ew errorWriter, s *webserver, err error) {
	ew.WriteError(http.StatusBadRequest, "content parse error")
	reportPayloadError(ctx, s, err)
}

func respondServerError(ctx Context, ew errorWriter, s *webserver, sourceURI, progHash, function, instID string, err error) {
	status := web.ErrorStatus(err)

	switch status {
	case http.StatusUnauthorized:
		ew.SetHeader("Www-Authenticate", fmt.Sprintf("%s realm=%q", web.AuthorizationTypeBearer, s.identity))

	case http.StatusTooManyRequests:
		if e := api.AsTooManyRequests(err); e != nil {
			if d := e.RetryAfter(); d > 0 {
				s := d / time.Second
				if s == 0 {
					s = 1
				}
				ew.SetHeader("Retry-After", strconv.Itoa(int(s)))
			}
		}

	case http.StatusInternalServerError:
		ew.WriteError(status, api.PublicErrorString(err, "internal error"))
		reportInternalError(ctx, s, sourceURI, progHash, function, instID, err)
		return
	}

	ew.WriteError(status, api.PublicErrorString(err, "unknown error"))
	reportRequestError(ctx, s, event.ErrorFailType(err), sourceURI, progHash, function, instID, err)
}

func webStatus(s *api.Status) (w web.Status) {
	w.State = s.State.String()
	if s.Cause != 0 {
		w.Cause = s.Cause.String()
	}
	if s.State == api.StateHalted || s.State == api.StateTerminated {
		w.Result = int(s.Result)
	}
	w.Error = s.Error
	return
}
