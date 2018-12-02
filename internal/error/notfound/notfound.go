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

// ErrFunction is public.
var ErrFunction function

type function struct{}

func (f function) Error() string       { return f.PublicError() }
func (f function) PublicError() string { return "function not found or type is incompatible" }
func (f function) NotFound()           {}
func (f function) FunctionNotFound()   {}
