// Copyright (c) 2020 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"fmt"
	"os"
)

func openAsOSFile(fd uintptr) (*os.File, error) {
	return os.Open(fmt.Sprintf("/proc/self/fd/%d", fd))
}
