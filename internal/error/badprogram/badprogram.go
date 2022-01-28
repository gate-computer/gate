// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package badprogram

import (
	"fmt"
)

type Error interface {
	error
	ProgramError() bool
}

// Errorf formats public information.
func Errorf(format string, args ...interface{}) error {
	return Err(fmt.Sprintf(format, args...))
}

// Err is a constant-compatible type.
type Err string

func (s Err) Error() string       { return string(s) }
func (s Err) PublicError() string { return string(s) }
func (s Err) ProgramError() bool  { return true }
