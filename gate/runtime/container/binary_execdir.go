// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build gateexecdir

package container

import (
	"os"
	"path"
)

func init() {
	if ExecDir == "" {
		if filename, err := os.Executable(); err == nil {
			ExecDir = path.Dir(filename)
		}
	}
}
