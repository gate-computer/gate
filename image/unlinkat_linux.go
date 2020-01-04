// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"fmt"
	"os"
	"syscall"
)

func unlinkat(dirfd uintptr, path string) (err error) {
	err = syscall.Unlinkat(int(dirfd), path)
	if err != nil {
		if err == syscall.ENOENT {
			err = os.ErrNotExist
			return
		}

		err = fmt.Errorf("unlinkat %d %q: %v", dirfd, path, err)
		return
	}

	return
}
