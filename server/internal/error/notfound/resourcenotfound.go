// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package notfound

import (
	"net/http"

	"gate.computer/gate/internal/error/grpc"
	"gate.computer/gate/server/event"
)

// ErrModule is public.
var ErrModule moduleError

type moduleError struct{}

func (f moduleError) Error() string        { return f.PublicError() }
func (f moduleError) PublicError() string  { return "module not found" }
func (f moduleError) NotFound() bool       { return true }
func (f moduleError) ModuleNotFound() bool { return true }
func (f moduleError) Status() int          { return http.StatusNotFound }
func (f moduleError) GRPCCode() int        { return grpc.NotFound }
func (f moduleError) Fail() event.FailType { return event.FailModuleNotFound }

// ErrInstance is public.
var ErrInstance instanceError

type instanceError struct{}

func (f instanceError) Error() string          { return f.PublicError() }
func (f instanceError) PublicError() string    { return "instance not found" }
func (f instanceError) NotFound() bool         { return true }
func (f instanceError) InstanceNotFound() bool { return true }
func (f instanceError) Status() int            { return http.StatusNotFound }
func (f instanceError) GRPCCode() int          { return grpc.NotFound }
func (f instanceError) Fail() event.FailType   { return event.FailInstanceNotFound }
