// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wasm

import (
	"errors"
	"fmt"
	"io"

	"gate.computer/gate/snapshot"
	"gate.computer/gate/trap"
	"gate.computer/internal/error/badprogram"
	"gate.computer/internal/manifest"
	"gate.computer/wag/binary"
	"gate.computer/wag/section"
)

const (
	minSnapshotVersion = 0
	maxServiceNameLen  = 127
)

// Custom WebAssembly sections.
const (
	SectionSnapshot = "gate.snapshot" // Must appear once before gate.export or gate.buffer section.
	SectionExport   = "gate.export"   // May appear in place of standard export section.
	SectionBuffer   = "gate.buffer"   // May appear once between code and stack sections.
	SectionStack    = "gate.stack"    // May appear once between buffer and data sections.
)

func ReadSnapshotSection(r section.Reader) (snap snapshot.Snapshot, readLen int, err error) {
	version, n, err := binary.Varuint64(r)
	if err != nil {
		return
	}
	readLen += n

	if version < minSnapshotVersion {
		err = badprogram.Error(fmt.Sprintf("unsupported snapshot version: %d", version))
		return
	}

	flags, n, err := binary.Varuint64(r)
	if err != nil {
		return
	}
	readLen += n
	snap.Flags = snapshot.Flags(flags)

	trapID, n, err := binary.Varuint32(r)
	if err != nil {
		return
	}
	readLen += n
	snap.Trap = trap.ID(trapID)

	result, n, err := binary.Varuint32(r)
	if err != nil {
		return
	}
	readLen += n
	snap.Result = int32(result)

	snap.MonotonicTime, n, err = binary.Varuint64(r)
	if err != nil {
		return
	}
	readLen += n

	numBreakpoints, n, err := binary.Varuint32(r)
	if err != nil {
		return
	}
	readLen += n
	if numBreakpoints > manifest.MaxBreakpoints {
		err = errors.New("snapshot has too many breakpoints")
		return
	}

	snap.Breakpoints = make([]uint64, numBreakpoints)
	for i := range snap.Breakpoints {
		snap.Breakpoints[i], n, err = binary.Varuint64(r)
		if err != nil {
			return
		}
		readLen += n
	}

	return
}

func ReadBufferSectionHeader(r section.Reader, length uint32) (bs snapshot.Buffers, readLen int, dataBuf []byte, err error) {
	// TODO: limit sizes and count

	inputSize, n, err := binary.Varuint32(r)
	if err != nil {
		return
	}
	readLen += n

	outputSize, n, err := binary.Varuint32(r)
	if err != nil {
		return
	}
	readLen += n

	serviceCount, n, err := binary.Varuint32(r)
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
			err = badprogram.Error("service name length out of bounds")
			return
		}

		b := make([]byte, nameLen)
		n, err = io.ReadFull(r, b)
		if err != nil {
			return
		}
		readLen += n
		bs.Services[i].Name = string(b)

		serviceSizes[i], n, err = binary.Varuint32(r)
		if err != nil {
			return
		}
		readLen += n

		// TODO: limit size

		dataSize += int64(serviceSizes[i])
	}

	if int64(readLen)+dataSize > int64(length) {
		err = badprogram.Error("invalid buffer section in wasm module")
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
