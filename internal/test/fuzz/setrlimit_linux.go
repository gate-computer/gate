// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuzz

import (
	"fmt"
	"syscall"
)

func setrlimit(resource int, rlim *syscall.Rlimit) (err error) {
	err = syscall.Setrlimit(resource, rlim)
	if err != nil {
		err = fmt.Errorf("setrlimit: %v", err)
		return
	}

	return
}
