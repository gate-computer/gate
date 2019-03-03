// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package imageutil

import (
	"os"
	"path"
)

const (
	fsRootDir    = "v0"
	fsProgramDir = fsRootDir + "/program"
)

func MustCleanFilesystem(dir string) {
	if err := os.RemoveAll(path.Join(dir, fsProgramDir)); err != nil {
		panic(err)
	}
}
