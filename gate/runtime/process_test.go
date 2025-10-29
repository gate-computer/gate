// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
)

func TestImageInfo(t *testing.T) {
	assert.Equal(t, unsafe.Sizeof(imageInfo{}), uintptr(imageInfoSize))
}
