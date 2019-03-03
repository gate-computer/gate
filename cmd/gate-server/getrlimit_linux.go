// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"syscall"
)

func getrlimit(resource int, rlim *syscall.Rlimit) (err error) {
	err = syscall.Getrlimit(resource, rlim)
	if err != nil {
		err = fmt.Errorf("getrlimit: %v", err)
		return
	}

	return
}
