// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package notfound

import (
	"github.com/tsavola/gate/internal/error/public"
)

type Error interface {
	public.Error
	NotFound()
}

type FunctionError interface {
	Error
	FunctionNotFound()
}

// Public function errors.
var (
	ErrFunction  = function("function not exported or it cannot be used as entry function")
	ErrSuspended = function("explicit entry function may not be specified for suspended program")
)

type function string

func (f function) Error() string       { return string(f) }
func (f function) PublicError() string { return string(f) }
func (f function) NotFound()           {}
func (f function) FunctionNotFound()   {}
