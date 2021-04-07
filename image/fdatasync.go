// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"fmt"
	"syscall"
)

func fdatasync(fd int) error {
	if err := syscall.Fdatasync(fd); err != nil {
		return fmt.Errorf("fdatasync: %w", err)
	}

	return nil
}
