// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func copyFileRange(rfd uintptr, roff *int64, wfd uintptr, woff *int64, length int) (err error) {
	for length != 0 {
		var n int

		n, err = unix.CopyFileRange(int(rfd), roff, int(wfd), woff, length, 0)
		if err != nil {
			err = fmt.Errorf("copy_file_range: %v", err)
			return
		}

		length -= n
	}

	return
}
