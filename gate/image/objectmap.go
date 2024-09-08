// Copyright (c) 2020 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"unsafe"

	"gate.computer/internal/file"
	"gate.computer/wag/object"
)

const callSiteSize = 8 // The struct size or layout will not change between minor versions.

func callSitesSize(m *object.CallMap) int {
	return len(m.CallSites) * callSiteSize
}

func callSitesBytes(m *object.CallMap) []byte {
	size := callSitesSize(m)
	if size == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(&m.CallSites[0])), size)
}

func funcAddrsSize(m *object.CallMap) int {
	return len(m.FuncAddrs) * 4
}

func funcAddrsBytes(m *object.CallMap) []byte {
	size := funcAddrsSize(m)
	if size == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(&m.FuncAddrs[0])), size)
}

func copyObjectMapTo(b []byte, m *object.CallMap) {
	copy(b, callSitesBytes(m))
	copy(b[callSitesSize(m):], funcAddrsBytes(m))
}

func writeObjectMapAt(f *file.File, m *object.CallMap, offset int64) error {
	return f.WriteVecAt([2][]byte{callSitesBytes(m), funcAddrsBytes(m)}, offset)
}
