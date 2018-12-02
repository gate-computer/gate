// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package failrequest

import (
	"fmt"

	"github.com/tsavola/gate/internal/error/badrequest"
	"github.com/tsavola/gate/server/event"
)

type Error interface {
	badrequest.Error
	FailRequestType() event.FailRequest_Type
}

// New public information.
func New(t event.FailRequest_Type, s string) Error {
	return &simple{t, s}
}

// Errorf formats public information.
func Errorf(t event.FailRequest_Type, format string, args ...interface{}) Error {
	return &simple{t, fmt.Sprintf(format, args...)}
}

type simple struct {
	t event.FailRequest_Type
	s string
}

func (s *simple) Error() string                           { return s.s }
func (s *simple) PublicError() string                     { return s.s }
func (s *simple) BadRequest()                             {}
func (s *simple) FailRequestType() event.FailRequest_Type { return s.t }

// Wrap an internal error and associate public information with it.
func Wrap(t event.FailRequest_Type, cause error, public string) Error {
	return &wrap{simple{t, public}, cause}
}

// Wrapf an internal error and associate public information with it.
func Wrapf(t event.FailRequest_Type, cause error, format string, args ...interface{}) Error {
	return &wrap{simple{t, fmt.Sprintf(format, args...)}, cause}
}

type wrap struct {
	simple
	cause error
}

func (w *wrap) Error() string { return w.cause.Error() }
func (w *wrap) Cause() error  { return w.cause }

// Tag an error as public information.
func Tag(t event.FailRequest_Type, cause error) Error {
	return &tag{t, cause}
}

type tag struct {
	t     event.FailRequest_Type
	cause error
}

func (t *tag) Error() string                           { return t.cause.Error() }
func (t *tag) PublicError() string                     { return t.cause.Error() }
func (t *tag) Cause() error                            { return t.cause }
func (t *tag) BadRequest()                             {}
func (t *tag) FailRequestType() event.FailRequest_Type { return t.t }
