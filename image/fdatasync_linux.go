// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"fmt"
	"syscall"
)

func fdatasync(fd uintptr) (err error) {
	err = syscall.Fdatasync(int(fd))
	if err != nil {
		err = fmt.Errorf("fdatasync: %v", err)
		return
	}

	return
}
