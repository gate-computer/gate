// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"fmt"

	"github.com/tsavola/gate/internal/file"
	"golang.org/x/sys/unix"
)

func memfdCreate(name string) (f *file.File, err error) {
	fd, err := unix.MemfdCreate(name, unix.MFD_CLOEXEC)
	if err != nil {
		err = fmt.Errorf("memfd_create: %v", err)
		return
	}

	f = file.New(fd)
	return
}
