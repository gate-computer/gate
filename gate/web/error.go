// Copyright (c) 2022 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package web

import (
	"errors"
	"net/http"

	werrors "gate.computer/wag/errors"
)

type statusError interface {
	error
	Status() int
}

// ErrorStatus returns HTTP response status code representing an error.
func ErrorStatus(err error) int {
	if e := statusError(nil); errors.As(err, &e) {
		if s := e.Status(); s != 0 {
			return s
		}
	}

	if werrors.AsModuleError(err) != nil {
		return http.StatusBadRequest
	}

	if werrors.AsResourceLimit(err) != nil {
		return http.StatusBadRequest
	}

	return http.StatusInternalServerError
}
