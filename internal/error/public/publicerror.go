// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package public

import (
	"net/http"

	werrors "gate.computer/wag/errors"
	"google.golang.org/grpc/codes"
)

type usageError struct {
	str    string
	status int
	code   codes.Code
}

func (e *usageError) Error() string       { return e.str }
func (e *usageError) PublicError() string { return e.str }
func (e *usageError) Status() int         { return e.status }
func (e *usageError) Code() codes.Code    { return e.code }

func InvalidArgument(s string) error {
	return &usageError{s, http.StatusBadRequest, codes.InvalidArgument}
}

func FailedPrecondition(s string) error {
	return &usageError{s, http.StatusConflict, codes.FailedPrecondition}
}

func Unimplemented(s string) error {
	return &usageError{s, http.StatusNotImplemented, codes.Unimplemented}
}

type internalError string

func (e internalError) Error() string       { return string(e) }
func (e internalError) PublicError() string { return string(e) }

func Internal(s string) error {
	return internalError(s)
}

// ErrorString returns err.PublicError() if err is a PublicError.  Otherwise
// the alternative is returned.
func ErrorString(err error, alternative string) string {
	if x, ok := err.(werrors.PublicError); ok {
		return x.PublicError()
	}
	return alternative
}
