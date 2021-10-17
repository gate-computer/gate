// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !gateexecdir
// +build !gateexecdir

package container

import (
	"os"

	config "gate.computer/gate/runtime/container"
)

func openExecutorBinary(c *config.Config) (*os.File, error) {
	return os.Open(os.DevNull)
}

func openLoaderBinary(c *config.Config) (*os.File, error) {
	return os.Open(os.DevNull)
}
