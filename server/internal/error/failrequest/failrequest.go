// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package failrequest

import (
	"errors"
	"fmt"
	"net/http"

	"gate.computer/gate/server/event"
	"google.golang.org/grpc/codes"
)

var metadata = [...]struct {
	status int
	code   codes.Code
}{
	event.FailUnsupported:        {http.StatusNotImplemented, codes.Unimplemented},
	event.FailClientDenied:       {http.StatusForbidden, codes.PermissionDenied},
	event.FailPayloadError:       {http.StatusBadRequest, codes.InvalidArgument},
	event.FailPrincipalKeyError:  {http.StatusBadRequest, codes.InvalidArgument},
	event.FailAuthMissing:        {http.StatusUnauthorized, codes.Unauthenticated},
	event.FailAuthInvalid:        {http.StatusBadRequest, codes.InvalidArgument},
	event.FailAuthExpired:        {http.StatusForbidden, codes.PermissionDenied},
	event.FailAuthReused:         {http.StatusForbidden, codes.PermissionDenied},
	event.FailAuthDenied:         {http.StatusForbidden, codes.PermissionDenied},
	event.FailScopeTooLarge:      {http.StatusBadRequest, codes.InvalidArgument},
	event.FailResourceDenied:     {http.StatusForbidden, codes.PermissionDenied},
	event.FailResourceLimit:      {http.StatusBadRequest, codes.ResourceExhausted},
	event.FailRateLimit:          {http.StatusTooManyRequests, codes.Unavailable},
	event.FailModuleNotFound:     {http.StatusNotFound, codes.NotFound},
	event.FailModuleHashMismatch: {http.StatusBadRequest, codes.InvalidArgument},
	event.FailModuleError:        {http.StatusBadRequest, codes.InvalidArgument},
	event.FailFunctionNotFound:   {http.StatusNotFound, codes.NotFound},
	event.FailProgramError:       {http.StatusBadRequest, codes.InvalidArgument},
	event.FailInstanceNotFound:   {http.StatusNotFound, codes.NotFound},
	event.FailInstanceIDInvalid:  {http.StatusBadRequest, codes.InvalidArgument},
	event.FailInstanceIDExists:   {http.StatusConflict, codes.AlreadyExists},
	event.FailInstanceStatus:     {http.StatusConflict, codes.FailedPrecondition},
	event.FailInstanceNoConnect:  {http.StatusConflict, codes.FailedPrecondition},
	event.FailInstanceDebugState: {http.StatusConflict, codes.FailedPrecondition},
}

type FailError interface {
	error
	FailType() event.FailType
}

func AsError(err error) FailError {
	var e FailError
	if errors.As(err, &e) && e.FailType() != 0 {
		return e
	}
	return nil
}

// Error with public information.
func Error(t event.FailType, s string) error {
	return &simple{t, s}
}

// Errorf formats public information.
func Errorf(t event.FailType, format string, args ...interface{}) error {
	return &simple{t, fmt.Sprintf(format, args...)}
}

type simple struct {
	t event.FailType
	s string
}

func (s *simple) Error() string            { return s.s }
func (s *simple) PublicError() string      { return s.s }
func (s *simple) FailType() event.FailType { return s.t }

func (s *simple) Status() (status int) {
	if i := int(s.t); i < len(metadata) {
		status = metadata[i].status
	}
	if status == 0 {
		status = http.StatusInternalServerError
	}
	return
}

func (s *simple) Code() (code codes.Code) {
	if i := int(s.t); i < len(metadata) {
		code = metadata[i].code
	}
	if code == 0 {
		code = codes.Unknown
	}
	return
}

// WrapError and associate public information with it.
func WrapError(t event.FailType, public string, cause error) error {
	return &wrapped{simple{t, public}, cause}
}

type wrapped struct {
	simple
	cause error
}

func (w *wrapped) Error() string { return w.cause.Error() }
func (w *wrapped) Unwrap() error { return w.cause }
