// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package public

import (
	werrors "gate.computer/wag/errors"
	"google.golang.org/grpc/codes"
)

type coded struct {
	str  string
	code codes.Code
}

func (e *coded) Error() string       { return e.str }
func (e *coded) PublicError() string { return e.str }
func (e *coded) Code() codes.Code    { return e.code }

func InvalidArgument(s string) error    { return &coded{s, codes.InvalidArgument} }
func FailedPrecondition(s string) error { return &coded{s, codes.FailedPrecondition} }
func Unimplemented(s string) error      { return &coded{s, codes.Unimplemented} }
func Internal(s string) error           { return &coded{s, codes.Internal} }

// ErrorString returns err.PublicError() if err is a PublicError.  Otherwise
// the alternative is returned.
func ErrorString(err error, alternative string) string {
	if x, ok := err.(werrors.PublicError); ok {
		return x.PublicError()
	}
	return alternative
}
