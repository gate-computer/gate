// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build gateexecdir

package container

import (
	"os"
	"path"
	"syscall"

	"gate.computer/gate/internal/container/common"
	config "gate.computer/gate/runtime/container"
)

func openBinary(c *config.Config, name string) (*os.File, error) {
	dir := c.ExecDir
	if dir == "" {
		dir = config.ExecDir
	}
	return openPath(path.Join(dir, name), syscall.O_NOFOLLOW)
}

func openExecutorBinary(c *config.Config) (*os.File, error) {
	return openBinary(c, common.ExecutorFilename)
}

func openLoaderBinary(c *config.Config) (*os.File, error) {
	return openBinary(c, common.LoaderFilename)
}
