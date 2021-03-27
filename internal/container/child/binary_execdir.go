// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build gateexecdir

package child

import (
	"syscall"

	"gate.computer/gate/internal/container/common"
)

var executorNameArg = common.ExecutorFilename

func setupBinaries() error {
	syscall.CloseOnExec(common.ExecutorFD)
	return nil
}
