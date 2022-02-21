// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package resourcelimit

import (
	"fmt"
	"net/http"

	"gate.computer/gate/internal/error/grpc"
)

// Error with public information.
func Error(s string) error {
	return errorType(s)
}

// Errorf formats public information.
func Errorf(format string, args ...interface{}) error {
	return errorType(fmt.Sprintf(format, args...))
}

type errorType string

func (s errorType) Error() string       { return string(s) }
func (s errorType) PublicError() string { return string(s) }
func (s errorType) ResourceLimit() bool { return true }
func (s errorType) Status() int         { return http.StatusBadRequest }
func (s errorType) GRPCCode() int       { return grpc.ResourceExhausted }
