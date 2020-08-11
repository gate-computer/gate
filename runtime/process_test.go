// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"testing"
	"unsafe"
)

func TestImageInfo(*testing.T) {
	var x imageInfo

	if unsafe.Sizeof(x) != imageInfoSize {
		panic("imageInfo size mismatch")
	}
}

func TestProcessKey(t *testing.T) {
	var a, b ProcessKey

	if a != b {
		t.Fail()
	}
}
