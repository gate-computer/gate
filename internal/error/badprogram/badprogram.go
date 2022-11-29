// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package badprogram

import (
	"errors"
	"fmt"
	"net/http"

	"gate.computer/internal/error/grpc"
)

// Error is public.
func Error(s string) error {
	return errorType(s)
}

// Errorf formats public information.
func Errorf(format string, args ...any) error {
	return errorType(fmt.Sprintf(format, args...))
}

type errorType string

func (s errorType) Error() string       { return string(s) }
func (s errorType) PublicError() string { return string(s) }
func (s errorType) ProgramError() bool  { return true }
func (s errorType) Status() int         { return http.StatusBadRequest }
func (s errorType) GRPCCode() int       { return grpc.InvalidArgument }

type programError interface {
	error
	ProgramError() bool
}

// Is a program error?
func Is(err error) bool {
	var e programError
	return errors.As(err, &e) && e.ProgramError()
}
