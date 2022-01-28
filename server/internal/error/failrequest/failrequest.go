// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package failrequest

import (
	"fmt"
	"net/http"

	"gate.computer/gate/server/event"
	"google.golang.org/grpc/codes"
)

var typeStatuses = [25]int{
	event.FailClientDenied:       http.StatusForbidden,
	event.FailPayloadError:       http.StatusBadRequest,
	event.FailPrincipalKeyError:  http.StatusBadRequest,
	event.FailAuthMissing:        http.StatusUnauthorized,
	event.FailAuthInvalid:        http.StatusBadRequest,
	event.FailAuthExpired:        http.StatusForbidden,
	event.FailAuthReused:         http.StatusForbidden,
	event.FailAuthDenied:         http.StatusForbidden,
	event.FailScopeTooLarge:      http.StatusBadRequest,
	event.FailResourceDenied:     http.StatusForbidden,
	event.FailResourceLimit:      http.StatusBadRequest,
	event.FailRateLimit:          http.StatusTooManyRequests,
	event.FailModuleNotFound:     http.StatusNotFound,
	event.FailModuleHashMismatch: http.StatusBadRequest,
	event.FailModuleError:        http.StatusBadRequest,
	event.FailFunctionNotFound:   http.StatusNotFound,
	event.FailProgramError:       http.StatusBadRequest,
	event.FailInstanceNotFound:   http.StatusNotFound,
	event.FailInstanceIDInvalid:  http.StatusBadRequest,
	event.FailInstanceIDExists:   http.StatusConflict,
	event.FailInstanceStatus:     http.StatusConflict,
	event.FailInstanceNoConnect:  http.StatusConflict,
	event.FailInstanceTransient:  http.StatusConflict,
	event.FailInstanceDebugger:   http.StatusConflict,
}

var typeCodes = [25]codes.Code{
	event.FailClientDenied:       codes.PermissionDenied,
	event.FailPayloadError:       codes.InvalidArgument,
	event.FailPrincipalKeyError:  codes.InvalidArgument,
	event.FailAuthMissing:        codes.Unauthenticated,
	event.FailAuthInvalid:        codes.InvalidArgument,
	event.FailAuthExpired:        codes.PermissionDenied,
	event.FailAuthReused:         codes.PermissionDenied,
	event.FailAuthDenied:         codes.PermissionDenied,
	event.FailScopeTooLarge:      codes.InvalidArgument,
	event.FailResourceDenied:     codes.PermissionDenied,
	event.FailResourceLimit:      codes.ResourceExhausted,
	event.FailRateLimit:          codes.Unavailable,
	event.FailModuleNotFound:     codes.NotFound,
	event.FailModuleHashMismatch: codes.InvalidArgument,
	event.FailModuleError:        codes.InvalidArgument,
	event.FailFunctionNotFound:   codes.NotFound,
	event.FailProgramError:       codes.InvalidArgument,
	event.FailInstanceNotFound:   codes.NotFound,
	event.FailInstanceIDInvalid:  codes.InvalidArgument,
	event.FailInstanceIDExists:   codes.AlreadyExists,
	event.FailInstanceStatus:     codes.FailedPrecondition,
	event.FailInstanceNoConnect:  codes.FailedPrecondition,
	event.FailInstanceTransient:  codes.FailedPrecondition,
	event.FailInstanceDebugger:   codes.FailedPrecondition,
}

type Error interface {
	error
	FailRequestType() event.FailRequest_Type
}

// New public information.
func New(t event.FailRequest_Type, s string) error {
	return &simple{t, s}
}

// Errorf formats public information.
func Errorf(t event.FailRequest_Type, format string, args ...interface{}) error {
	return &simple{t, fmt.Sprintf(format, args...)}
}

type simple struct {
	t event.FailRequest_Type
	s string
}

func (s *simple) Error() string                           { return s.s }
func (s *simple) PublicError() string                     { return s.s }
func (s *simple) FailRequestType() event.FailRequest_Type { return s.t }

func (s *simple) Status() int {
	c := typeStatuses[s.t]
	if c == 0 {
		return http.StatusInternalServerError
	}
	return c
}

func (s *simple) Code() codes.Code {
	c := typeCodes[s.t]
	if c == 0 {
		return codes.Unknown
	}
	return c
}

// Wrap an internal error and associate public information with it.
func Wrap(t event.FailRequest_Type, cause error, public string) error {
	return &wrapped{simple{t, public}, cause}
}

type wrapped struct {
	simple
	cause error
}

func (w *wrapped) Error() string { return w.cause.Error() }
func (w *wrapped) Unwrap() error { return w.cause }
