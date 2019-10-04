// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"testing"
	"unsafe"

	internal "github.com/tsavola/gate/internal/executable"
)

func TestStackVars(*testing.T) {
	var x stackVars

	if unsafe.Sizeof(x) != internal.StackVarsSize {
		panic("stackVars size mismatch")
	}
}
