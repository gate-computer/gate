// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package public

import (
	werrors "gate.computer/wag/errors"
)

// Err is a constant-compatible type.
type Err string

func (s Err) Error() string       { return string(s) }
func (s Err) PublicError() string { return string(s) }

// Error returns err.PublicError() if err is an Error.  Otherwise the
// alternative is returned.
func Error(err error, alternative string) string {
	if x, ok := err.(werrors.PublicError); ok {
		return x.PublicError()
	}
	return alternative
}
