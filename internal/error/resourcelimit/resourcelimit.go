// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package resourcelimit

import (
	"fmt"

	werrors "github.com/tsavola/wag/errors"
)

type Error = werrors.ResourceLimit

// New error with public information.
func New(s string) error {
	return simple(s)
}

// Errorf formats public information.
func Errorf(format string, args ...interface{}) error {
	return simple(fmt.Sprintf(format, args...))
}

type simple string

func (s simple) Error() string       { return string(s) }
func (s simple) PublicError() string { return string(s) }
func (s simple) ResourceLimit()      {}
