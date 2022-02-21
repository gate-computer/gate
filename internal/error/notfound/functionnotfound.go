// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package notfound

import (
	"errors"
	"net/http"

	"gate.computer/gate/internal/error/grpc"
)

// Public function errors.
var (
	ErrFunction  error = function("function not exported or it cannot be used as entry function")
	ErrStart     error = function("entry function may not be specified for program with _start function")
	ErrSuspended error = function("entry function may not be specified for suspended program")
)

type function string

func (f function) Error() string          { return string(f) }
func (f function) PublicError() string    { return string(f) }
func (f function) NotFound() bool         { return true }
func (f function) FunctionNotFound() bool { return true }
func (f function) Status() int            { return http.StatusNotFound }
func (f function) GRPCCode() int          { return grpc.NotFound }

type functionNotFound interface {
	error
	FunctionNotFound() bool
}

func IsFunction(err error) bool {
	var e functionNotFound
	return errors.As(err, &e) && e.FunctionNotFound()
}
