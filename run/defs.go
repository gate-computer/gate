package run

import (
	"encoding/binary"
)

// some of these are also defined in defs.h, run.js and work.js

const (
	RODataAddr = 0x10000
)

const (
	abiVersion    = 0
	maxPacketSize = 0x10000 // coincides with default pipe buffer size on Linux
	maxServices   = 100

	headerSize = 8
)

var (
	endian = binary.LittleEndian
)
