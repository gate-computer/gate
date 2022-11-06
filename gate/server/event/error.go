// Copyright (c) 2022 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package event

import (
	"errors"

	"gate.computer/internal/error/badprogram"
	"gate.computer/internal/error/notfound"
	werrors "gate.computer/wag/errors"
)

type failError interface {
	error
	FailType() FailType
}

// ErrorFailType returns FailRequest event failure type representing an error.
func ErrorFailType(err error) FailType {
	if e := failError(nil); errors.As(err, &e) {
		if t := e.FailType(); t != 0 {
			return t
		}
	}

	if badprogram.Is(err) {
		return FailProgramError
	}

	if notfound.IsFunction(err) {
		return FailFunctionNotFound
	}

	if werrors.AsModuleError(err) != nil {
		return FailModuleError
	}

	if werrors.AsResourceLimit(err) != nil {
		return FailResourceLimit
	}

	return FailInternal
}
