// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sys

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

func ClearCaps() error {
	var (
		hdr  = unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}
		data = [2]unix.CapUserData{}
	)

	_, _, errno := syscall.AllThreadsSyscall(
		unix.SYS_CAPSET,
		uintptr(unsafe.Pointer(&hdr)),
		uintptr(unsafe.Pointer(&data[0])),
		0,
	)
	if errno != 0 {
		return fmt.Errorf("clearing all capabilities (capset): %w", errno)
	}

	if err := unix.Prctl(unix.PR_CAP_AMBIENT, unix.PR_CAP_AMBIENT_CLEAR_ALL, 0, 0, 0); err != nil {
		return fmt.Errorf("clearing ambient capabilities (prctl): %w", err)
	}

	return nil
}
