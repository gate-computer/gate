// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package failrequest

import (
	"errors"
	"fmt"
	"net/http"

	"gate.computer/gate/internal/error/grpc"
	"gate.computer/gate/server/event"
)

var metadata = [...]struct {
	status int
	code   int
}{
	event.FailUnsupported:        {http.StatusNotImplemented, grpc.Unimplemented},
	event.FailClientDenied:       {http.StatusForbidden, grpc.PermissionDenied},
	event.FailPayloadError:       {http.StatusBadRequest, grpc.InvalidArgument},
	event.FailPrincipalKeyError:  {http.StatusBadRequest, grpc.InvalidArgument},
	event.FailAuthMissing:        {http.StatusUnauthorized, grpc.Unauthenticated},
	event.FailAuthInvalid:        {http.StatusBadRequest, grpc.InvalidArgument},
	event.FailAuthExpired:        {http.StatusForbidden, grpc.PermissionDenied},
	event.FailAuthReused:         {http.StatusForbidden, grpc.PermissionDenied},
	event.FailAuthDenied:         {http.StatusForbidden, grpc.PermissionDenied},
	event.FailScopeTooLarge:      {http.StatusBadRequest, grpc.InvalidArgument},
	event.FailResourceDenied:     {http.StatusForbidden, grpc.PermissionDenied},
	event.FailResourceLimit:      {http.StatusBadRequest, grpc.ResourceExhausted},
	event.FailRateLimit:          {http.StatusTooManyRequests, grpc.Unavailable},
	event.FailModuleNotFound:     {http.StatusNotFound, grpc.NotFound},
	event.FailModuleHashMismatch: {http.StatusBadRequest, grpc.InvalidArgument},
	event.FailModuleError:        {http.StatusBadRequest, grpc.InvalidArgument},
	event.FailFunctionNotFound:   {http.StatusNotFound, grpc.NotFound},
	event.FailProgramError:       {http.StatusBadRequest, grpc.InvalidArgument},
	event.FailInstanceNotFound:   {http.StatusNotFound, grpc.NotFound},
	event.FailInstanceIDInvalid:  {http.StatusBadRequest, grpc.InvalidArgument},
	event.FailInstanceIDExists:   {http.StatusConflict, grpc.AlreadyExists},
	event.FailInstanceStatus:     {http.StatusConflict, grpc.FailedPrecondition},
	event.FailInstanceNoConnect:  {http.StatusConflict, grpc.FailedPrecondition},
	event.FailInstanceDebugState: {http.StatusConflict, grpc.FailedPrecondition},
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

func (s *simple) Code() (code int) {
	if i := int(s.t); i < len(metadata) {
		code = metadata[i].code
	}
	if code == 0 {
		code = grpc.Unknown
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
