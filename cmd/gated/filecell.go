// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"os"

	dbus "github.com/godbus/dbus/v5"
)

type fileCell struct {
	f *os.File
}

func newFileCell(fd dbus.UnixFD, name string) *fileCell {
	return &fileCell{
		f: os.NewFile(uintptr(fd), name),
	}
}

func (c *fileCell) steal() (f *os.File) {
	f = c.f
	if f == nil {
		panic("file cell is empty")
	}
	c.f = nil
	return
}

func (c *fileCell) Close() (err error) {
	if c.f != nil {
		err = c.f.Close()
		c.f = nil
	}
	return
}
