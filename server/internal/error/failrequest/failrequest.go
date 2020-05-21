// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package failrequest

import (
	"fmt"

	"gate.computer/gate/server/event"
)

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
