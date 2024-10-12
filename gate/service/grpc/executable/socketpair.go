// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package executable

import (
	"fmt"
	"os"
	"syscall"
)

// socketFilePair returns a blocking (f1) and a pollable (f2) file.
func socketFilePair(flags int) (f1, f2 *os.File, err error) {
	p, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC|flags, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("socketpair: %w", err)
	}

	if err := syscall.SetNonblock(p[1], true); err != nil {
		return nil, nil, fmt.Errorf("set nonblock: %w", err)
	}

	f1 = os.NewFile(uintptr(p[0]), "unix")
	f2 = os.NewFile(uintptr(p[1]), "unix")
	return f1, f2, nil
}
