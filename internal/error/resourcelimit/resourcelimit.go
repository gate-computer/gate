// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package resourcelimit

import (
	"fmt"

	"github.com/tsavola/gate/internal/error/public"
)

type Error interface {
	public.Error
	Forbidden()
	ResourceLimit()
}

// New error with public information.
func New(s string) Error {
	return simple(s)
}

// Errorf formats public information.
func Errorf(format string, args ...interface{}) Error {
	return simple(fmt.Sprintf(format, args...))
}

type simple string

func (s simple) Error() string       { return string(s) }
func (s simple) PublicError() string { return string(s) }
func (s simple) Forbidden()          {}
func (s simple) ResourceLimit()      {}
