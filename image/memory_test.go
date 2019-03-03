// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"testing"
)

func TestMemory(*testing.T) {
	var _ InstanceStorage = Memory
	var _ ProgramStorage = Memory
	var _ LocalStorage = Memory
}
