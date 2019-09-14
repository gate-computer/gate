// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package badprogram

import (
	"fmt"

	"github.com/tsavola/gate/internal/error/badrequest"
)

type Error interface {
	badrequest.Error
	BadProgram()
}

// Errorf formats public information.
func Errorf(format string, args ...interface{}) badrequest.Error {
	return Err(fmt.Sprintf(format, args...))
}

// Err is a constant-compatible type.
type Err string

func (s Err) Error() string       { return string(s) }
func (s Err) PublicError() string { return string(s) }
func (s Err) BadRequest()         {}
func (s Err) BadProgram()         {}
