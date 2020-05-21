// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"fmt"
	"syscall"

	"gate.computer/gate/internal/file"
)

func socketPipe() (r *file.Ref, w *file.File, err error) {
	p, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC, 0)
	if err != nil {
		err = fmt.Errorf("socketpair: %v", err)
		return
	}
	defer func() {
		if err != nil {
			syscall.Close(p[0])
			syscall.Close(p[1])
		}
	}()

	err = syscall.Shutdown(p[0], syscall.SHUT_WR)
	if err != nil {
		err = fmt.Errorf("shutdown: %v", err)
		return
	}

	r = file.NewRef(p[0])
	w = file.New(p[1])
	return
}
