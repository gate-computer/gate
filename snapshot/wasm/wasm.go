// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wasm

import (
	"io"

	"github.com/tsavola/gate/snapshot"
	"github.com/tsavola/wag/section"
)

// Custom WebAssembly sections.
const (
	ServiceSection = "gate.service" // May appear once after code section.
	IOSection      = "gate.io"      // May appear once after service section.
	BufferSection  = "gate.buffer"  // May appear once after io section.
	StackSection   = "gate.stack"   // May appear once between buffer and data sections.
)

func ReadServiceSection(r section.Reader, length uint32, newError func(string) error,
) (services []snapshot.Service, buf []byte, err error) {
	var readLen int

	count, n, err := readVaruint32(r, newError)
	if err != nil {
		return
	}
	readLen += n

	// TODO: validate count

	services = make([]snapshot.Service, count)
	sizes := make([]uint32, count)

	var totalSize uint64

	for i := range services {
		var nameLen byte

		nameLen, err = r.ReadByte()
		if err != nil {
			return
		}
		readLen++

		// TODO: validate nameLen

		b := make([]byte, nameLen)
		n, err = io.ReadFull(r, b)
		if err != nil {
			return
		}
		readLen += n
		services[i].Name = string(b)

		sizes[i], n, err = readVaruint32(r, newError)
		if err != nil {
			return
		}
		readLen += n

		// TODO: validate size

		totalSize += uint64(sizes[i])
	}

	if uint64(readLen) != uint64(length) {
		err = newError("invalid service section in wasm module")
		return
	}

	// TODO: validate totalSize

	buf = make([]byte, totalSize)
	return
}

func ReadIOSection(r section.Reader, length uint32, newError func(string) error,
) (inputBuf, outputBuf []byte, err error) {
	inputSize, n1, err := readVaruint32(r, newError)
	if err != nil {
		return
	}

	outputSize, n2, err := readVaruint32(r, newError)
	if err != nil {
		return
	}

	// TODO: validate sizes

	if uint64(n1+n2) != uint64(length) {
		err = newError("invalid io section in wasm module")
		return
	}

	inputBuf = make([]byte, inputSize)
	outputBuf = make([]byte, outputSize)
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
