// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package publicerror

import (
	"fmt"
)

type PublicError interface {
	error
	Cause() error
	PublicError() string
	Internal() string // returns subsystem name if the error is internal
}

// New is like errors.New, but the result can be published.  The error is not
// our fault.
func New(text string) PublicError {
	return simple{text}
}

// Errorf is like fmt.Errorf, but the result can be published.  The error is
// not our fault.
func Errorf(format string, args ...interface{}) PublicError {
	return simple{fmt.Sprintf(format, args...)}
}

type simple struct {
	text string
}

func (err simple) Cause() error        { return err }
func (err simple) Error() string       { return err.text }
func (err simple) PublicError() string { return err.text }
func (simple) Internal() string        { return "" }

// Tag an existing error as safe for publication.  The error is not our fault.
func Tag(err error) PublicError {
	return safe{err}
}

type safe struct {
	error
}

func (safe safe) Cause() error        { return safe.error }
func (safe safe) PublicError() string { return safe.Error() }
func (safe) Internal() string         { return "" }

// Internal function failed with this cause which can be published.
func Internal(subsystem string, cause error) PublicError {
	return &failure{subsystem, cause}
}

type failure struct {
	subsystem string
	cause     error
}

func (fail *failure) Cause() error        { return fail.cause }
func (fail *failure) Error() string       { return fail.cause.Error() }
func (fail *failure) PublicError() string { return fail.cause.Error() }
func (fail *failure) Internal() string    { return fail.subsystem }

// Shutdown error with an unpublishable cause.
func Shutdown(subsystem string, cause error) PublicError {
	return &dual{subsystem, cause}
}

type dual struct {
	subsystem string
	cause     error
}

func (err *dual) Cause() error     { return err.cause }
func (err *dual) Error() string    { return err.cause.Error() }
func (*dual) PublicError() string  { return "Shutting down" }
func (err *dual) Internal() string { return err.subsystem }
