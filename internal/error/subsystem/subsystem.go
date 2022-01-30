// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package subsystem

import (
	"errors"
)

type subsystemError interface {
	error
	Subsystem() string
}

func Get(err error) string {
	var e subsystemError
	if errors.As(err, &e) {
		return e.Subsystem()
	}
	return ""
}
