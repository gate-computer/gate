// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cred

import (
	"syscall"
)

func getuid() uint { return uint(syscall.Getuid()) }
func getgid() uint { return uint(syscall.Getgid()) }
