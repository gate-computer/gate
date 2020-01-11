// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package public

import (
	werrors "github.com/tsavola/wag/errors"
)

// Error returns err.PublicError() if err is an Error.  Otherwise the
// alternative is returned.
func Error(err error, alternative string) string {
	if x, ok := err.(werrors.PublicError); ok {
		return x.PublicError()
	}
	return alternative
}
