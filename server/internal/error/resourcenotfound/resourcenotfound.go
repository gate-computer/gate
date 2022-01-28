// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package resourcenotfound

import (
	"gate.computer/gate/internal/error/notfound"
)

type ModuleError interface {
	notfound.Error
	ModuleNotFound() bool
}

// ErrModule is public.
var ErrModule module

type module struct{}

func (f module) Error() string        { return f.PublicError() }
func (f module) PublicError() string  { return "module not found" }
func (f module) NotFound() bool       { return true }
func (f module) ModuleNotFound() bool { return true }

type InstanceError interface {
	notfound.Error
	InstanceNotFound() bool
}

// ErrInstance is public.
var ErrInstance instance

type instance struct{}

func (f instance) Error() string          { return f.PublicError() }
func (f instance) PublicError() string    { return "instance not found" }
func (f instance) NotFound() bool         { return true }
func (f instance) InstanceNotFound() bool { return true }
