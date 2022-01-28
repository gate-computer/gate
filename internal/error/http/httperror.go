// Copyright (c) 2022 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package http

import (
	"errors"
	"net/http"

	werrors "gate.computer/wag/errors"
)

type statusError interface {
	error
	Status() int
}

func Status(err error) int {
	if x := statusError(nil); errors.As(err, &x) {
		if s := x.Status(); s != 0 {
			return s
		}
		return http.StatusInternalServerError // Defensive measure.
	}

	if werrors.AsModuleError(err) != nil {
		return http.StatusBadRequest
	}

	if werrors.AsResourceLimit(err) != nil {
		return http.StatusBadRequest
	}

	return http.StatusInternalServerError
}
