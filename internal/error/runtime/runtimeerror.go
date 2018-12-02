// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"fmt"
)

type Error struct {
	Define string
	Subsys string
	Text   string
}

func (e *Error) Error() string     { return e.Text }
func (e *Error) Subsystem() string { return e.Subsys }

func ExecutorError(code int) error {
	if code < len(ExecutorErrors) {
		e := &ExecutorErrors[code]
		if e.Define != "" {
			return e
		}
	}

	return fmt.Errorf("unknown exit code %d", code)
}

func ProcessError(code int) error {
	if code < len(ProcessErrors) {
		e := &ProcessErrors[code]
		if e.Define != "" {
			return e
		}
	}

	return fmt.Errorf("unknown runtime process exit code %d", code)
}
