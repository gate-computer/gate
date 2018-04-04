// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package publicerror

import "fmt"

type PublicError interface {
	error
	PrivateErr() error
	PublicError() string
	Internal() string
}

// New is like errors.New, but the result can be published.  The error is not
// our fault.
func New(text string) PublicError {
	return simple{text}
}

// Errof is like fmt.Errorf, but the result can be published.  The error is not
// our fault.
func Errorf(format string, args ...interface{}) PublicError {
	return simple{fmt.Sprintf(format, args...)}
}

type simple struct {
	text string
}

func (err simple) Error() string       { return err.text }
func (err simple) PrivateErr() error   { return err }
func (err simple) PublicError() string { return err.text }
func (simple) Internal() string        { return "" }

// Tag an existing error as safe for publication.  The error is not our fault.
func Tag(err error) PublicError {
	return safe{err}
}

type safe struct {
	err error
}

func (safe safe) Error() string       { return safe.err.Error() }
func (safe safe) PrivateErr() error   { return safe.err }
func (safe safe) PublicError() string { return safe.err.Error() }
func (safe) Internal() string         { return "" }

// Internal function failed with this internal error which can be published.
func Internal(subsystem string, err error) PublicError {
	return &failure{subsystem, err}
}

type failure struct {
	subsystem string
	err       error
}

func (fail *failure) Error() string       { return fail.err.Error() }
func (fail *failure) PrivateErr() error   { return fail.err }
func (fail *failure) PublicError() string { return fail.err.Error() }
func (fail *failure) Internal() string    { return fail.subsystem }

// Shutdown error with private details.
func Shutdown(subsystem string, private error) PublicError {
	return &dual{subsystem, private}
}

type dual struct {
	subsystem string
	private   error
}

func (err *dual) Error() string     { return err.private.Error() }
func (err *dual) PrivateErr() error { return err.private }
func (*dual) PublicError() string   { return "Shutting down" }
func (err *dual) Internal() string  { return err.subsystem }
