// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"fmt"
	"syscall"
)

// rename is like os.Rename, but without the extraneous lstat call.
func rename(oldPath, newPath string) (err error) {
	err = syscall.Rename(oldPath, newPath)
	if err != nil {
		err = fmt.Errorf("rename %q to %q: %v", oldPath, newPath, err)
		return
	}

	return
}
