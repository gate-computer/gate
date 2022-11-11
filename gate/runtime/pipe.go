// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"fmt"
	"os"
	"syscall"

	"gate.computer/internal/file"
)

func pipe2(flags int) (r *os.File, w *file.File, err error) {
	var p [2]int

	if err := syscall.Pipe2(p[:], syscall.O_CLOEXEC|flags); err != nil {
		return nil, nil, fmt.Errorf("pipe2: %w", err)
	}

	r = os.NewFile(uintptr(p[0]), "|0")
	w = file.New(p[1])
	return r, w, nil
}
