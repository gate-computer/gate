// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package executable

import (
	"encoding/binary"
	"os"
)

const (
	MinTextAddr  = 0x000400000000
	MaxTextAddr  = 0x2aa700000000
	MinHeapAddr  = 0x2aa900000000
	MaxHeapAddr  = 0x554b00000000
	MinStackAddr = 0x554d00000000
	MaxStackAddr = 0x7ff000000000
)

var PageSize = os.Getpagesize()

func RandAddr(minAddr, maxAddr uint64, b []byte) uint64 {
	minPage := minAddr / uint64(PageSize)
	maxPage := maxAddr / uint64(PageSize)
	page := minPage + uint64(binary.LittleEndian.Uint32(b))%(maxPage-minPage)
	return page * uint64(PageSize)
}
