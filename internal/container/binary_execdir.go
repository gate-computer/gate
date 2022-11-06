// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build gateexecdir
// +build gateexecdir

package container

import (
	"fmt"
	"os"
	"path"
	"syscall"

	"gate.computer/internal/container/common"
	config "gate.computer/gate/runtime/container"
	"golang.org/x/sys/unix"
)

func openBinary(c *config.Config, name string) (*os.File, error) {
	dir := c.ExecDir
	if dir == "" {
		dir = config.ExecDir
	}
	filename := path.Join(dir, name)

	fd, err := syscall.Open(filename, unix.O_CLOEXEC|unix.O_PATH, 0)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", filename, err)
	}

	return os.NewFile(uintptr(fd), filename), nil
}

func openExecutorBinary(c *config.Config) (*os.File, error) {
	return openBinary(c, common.ExecutorFilename)
}

func openLoaderBinary(c *config.Config) (*os.File, error) {
	return openBinary(c, common.LoaderFilename)
}
