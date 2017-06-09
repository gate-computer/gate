package run

import (
	"encoding/binary"
)

// some of these are also defined in defs.h, run.js and work.js

const (
	RODataAddr = 0x10000

	minTextAddr = 0x000300000000 + 0x100000000
	maxTextAddr = 0x400000000000 - 0x100000000

	minHeapAddr = 0x400000000000 + 0x100000000
	maxHeapAddr = 0x7f0000000000 - 0x200000000
)

const (
	abiVersion    = 0
	maxPacketSize = 0x10000 // coincides with default pipe buffer size on Linux
	maxServices   = 100

	packetHeaderSize = 8
)

var (
	endian = binary.LittleEndian
)
