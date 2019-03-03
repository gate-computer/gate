// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"fmt"
	"syscall"
)

func sendmsg(fd uintptr, p, oob []byte, to syscall.Sockaddr, flags int) (err error) {
	err = syscall.Sendmsg(int(fd), p, oob, to, flags)
	if err != nil {
		err = fmt.Errorf("sendmsg: %v", err)
		return
	}

	return
}

func unixRights(fds ...int) []byte {
	return syscall.UnixRights(fds...)
}
