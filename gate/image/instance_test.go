// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"testing"
	"unsafe"

	internal "gate.computer/internal/executable"
	"github.com/stretchr/testify/assert"
)

func TestStackVars(t *testing.T) {
	assert.Equal(t, unsafe.Sizeof(stackVars{}), uintptr(internal.StackVarsSize))
}
