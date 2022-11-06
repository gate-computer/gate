// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build gateexecdir
// +build gateexecdir

package child

import (
	"syscall"

	"gate.computer/internal/container/common"
)

func setupBinaries() error {
	syscall.CloseOnExec(common.ExecutorFD)
	return nil
}
