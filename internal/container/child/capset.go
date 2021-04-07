// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package child

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func threadCapsetZero() error {
	var (
		hdr  = unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}
		data = [2]unix.CapUserData{}
	)

	if err := unix.Capset(&hdr, &data[0]); err != nil {
		return fmt.Errorf("clearing all capabilities (capset): %w", err)
	}

	return nil
}
