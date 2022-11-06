// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"os"

	"gate.computer/gate/runtime/container"
)

const MaxProcs = 16384 // Per Executor.

type Config struct {
	MaxProcs     int
	ConnFile     *os.File
	DaemonSocket string           // Applicable if ConnFile is not set.
	Container    container.Config // Applicable if ConnFile and DaemonSocket are not set.
	ErrorLog     Logger
}

var DefaultConfig = Config{
	MaxProcs:  MaxProcs,
	Container: container.DefaultConfig,
}
