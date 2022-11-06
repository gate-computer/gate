// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"fmt"
	"syscall"
)

func ftruncate(fd int, length int64) error {
	if err := syscall.Ftruncate(fd, length); err != nil {
		return fmt.Errorf("ftruncate: %w", err)
	}

	return nil
}
