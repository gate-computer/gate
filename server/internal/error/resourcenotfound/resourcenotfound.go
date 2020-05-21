// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package resourcenotfound

import (
	"gate.computer/gate/internal/error/notfound"
)

type ModuleError interface {
	notfound.Error
	ModuleNotFound()
}

// ErrModule is public.
var ErrModule module

type module struct{}

func (f module) Error() string       { return f.PublicError() }
func (f module) PublicError() string { return "module not found" }
func (f module) NotFound()           {}
func (f module) ModuleNotFound()     {}

type InstanceError interface {
	notfound.Error
	InstanceNotFound()
}

// ErrInstance is public.
var ErrInstance instance

type instance struct{}

func (f instance) Error() string       { return f.PublicError() }
func (f instance) PublicError() string { return "instance not found" }
func (f instance) NotFound()           {}
func (f instance) InstanceNotFound()   {}
