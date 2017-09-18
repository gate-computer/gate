// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package run

import (
	"encoding/binary"
)

// some of these are also defined in defs.h, run.js and work.js

const (
	EntrySymbol = "_start"
	RODataAddr  = 0x10000

	minTextAddr  = 0x000400000000
	maxTextAddr  = 0x2aa700000000
	minHeapAddr  = 0x2aa900000000
	maxHeapAddr  = 0x554b00000000
	minStackAddr = 0x554d00000000
	maxStackAddr = 0x7ff000000000

	signalStackReserve = 0x600 // TODO
)

const (
	abiVersion    = 0
	pipeBufSize   = 0x10000 // Linux default
	maxPacketSize = pipeBufSize / 2
	maxServices   = 100

	magicNumber = 0x7e1c5d67
)

var (
	endian = binary.LittleEndian
)
