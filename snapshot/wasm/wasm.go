// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wasm

import (
	"io"

	"github.com/tsavola/gate/snapshot"
	"github.com/tsavola/wag/section"
)

const maxServiceNameLen = 127

// Custom WebAssembly sections.
const (
	SectionSnapshot = "gate.snapshot" // Must appear once somewhere before buffer section.
	SectionBuffer   = "gate.buffer"   // May appear once between code and stack sections.
	SectionStack    = "gate.stack"    // May appear once between buffer and data sections.
)

func ReadBufferSectionHeader(r section.Reader, length uint32, newError func(string) error,
) (bs snapshot.Buffers, readLen int, dataBuf []byte, err error) {
	flags, n, err := readVaruint32(r, newError)
	if err != nil {
		return
	}
	readLen += n

	bs.Flags = snapshot.Flags(flags)

	// TODO: limit sizes and count

	inputSize, n, err := readVaruint32(r, newError)
	if err != nil {
		return
	}
	readLen += n

	outputSize, n, err := readVaruint32(r, newError)
	if err != nil {
		return
	}
	readLen += n

	serviceCount, n, err := readVaruint32(r, newError)
	if err != nil {
		return
	}
	readLen += n

	dataSize := int64(inputSize) + int64(outputSize)

	bs.Services = make([]snapshot.Service, serviceCount)
	serviceSizes := make([]uint32, serviceCount)

	for i := range bs.Services {
		var nameLen byte

		nameLen, err = r.ReadByte()
		if err != nil {
			return
		}
		readLen++

		if nameLen == 0 || nameLen > maxServiceNameLen {
			err = newError("service name length out of bounds")
			return
		}

		b := make([]byte, nameLen)
		n, err = io.ReadFull(r, b)
		if err != nil {
			return
		}
		readLen += n
		bs.Services[i].Name = string(b)

		serviceSizes[i], n, err = readVaruint32(r, newError)
		if err != nil {
			return
		}
		readLen += n

		// TODO: limit size

		dataSize += int64(serviceSizes[i])
	}

	if int64(readLen)+dataSize > int64(length) {
		err = newError("invalid buffer section in wasm module")
		return
	}

	dataBuf = make([]byte, dataSize)
	b := dataBuf

	bs.Input = b[:inputSize:inputSize]
	b = b[inputSize:]

	bs.Output = b[:outputSize:outputSize]
	b = b[outputSize:]

	for i, size := range serviceSizes {
		bs.Services[i].Buffer = b[:size:size]
		b = b[size:]
	}

	return
}

func readVaruint32(r section.Reader, newError func(string) error) (x uint32, n int, err error) {
	var shift uint
	for n = 1; ; n++ {
		var b byte
		b, err = r.ReadByte()
		if err != nil {
			return
		}
		if b < 0x80 {
			if n > 5 || n == 5 && b > 0xf {
				err = newError("varuint32 is too large")
				return
			}
			x |= uint32(b) << shift
			return
		}
		x |= (uint32(b) & 0x7f) << shift
		shift += 7
	}
}
