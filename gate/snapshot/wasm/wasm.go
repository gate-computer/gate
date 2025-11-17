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

func ReadSnapshotSection(r section.Reader) (snap *snapshot.Snapshot, readLen int, err error) {
	version, n, err := binary.Varuint64(r)
	readLen += n
	if err != nil {
		return nil, readLen, err
	}
	if version < minSnapshotVersion {
		return nil, readLen, badprogram.Error(fmt.Sprintf("unsupported snapshot version: %d", version))
	}

	flags, n, err := binary.Varuint64(r)
	readLen += n
	if err != nil {
		return nil, readLen, err
	}
	snap = &snapshot.Snapshot{
		Final: flags&1 != 0,
	}

	trapID, n, err := binary.Varuint32(r)
	readLen += n
	if err != nil {
		return nil, readLen, err
	}
	snap.Trap = trap.ID(trapID)

	result, n, err := binary.Varuint32(r)
	readLen += n
	if err != nil {
		return nil, readLen, err
	}
	snap.Result = int32(result)

	snap.MonotonicTime, n, err = binary.Varuint64(r)
	readLen += n
	if err != nil {
		return nil, readLen, err
	}

	numBreakpoints, n, err := binary.Varuint32(r)
	readLen += n
	if err != nil {
		return nil, readLen, err
	}
	if numBreakpoints > snapshot.MaxBreakpoints {
		return nil, readLen, errors.New("snapshot has too many breakpoints")
	}

	snap.Breakpoints = make([]uint64, numBreakpoints)
	for i := range snap.Breakpoints {
		snap.Breakpoints[i], n, err = binary.Varuint64(r)
		readLen += n
		if err != nil {
			return nil, readLen, err
		}
	}

	return snap, readLen, nil
}

func ReadBufferSectionHeader(r section.Reader, length uint32) (bs *snapshot.Buffers, readLen int, dataBuf []byte, err error) {
	// TODO: limit sizes and count

	inputSize, n, err := binary.Varuint32(r)
	readLen += n
	if err != nil {
		return nil, readLen, nil, err
	}

	outputSize, n, err := binary.Varuint32(r)
	readLen += n
	if err != nil {
		return nil, readLen, nil, err
	}

	serviceCount, n, err := binary.Varuint32(r)
	readLen += n
	if err != nil {
		return nil, readLen, nil, err
	}

	dataSize := int64(inputSize) + int64(outputSize)

	bs = &snapshot.Buffers{
		Services: make([]*snapshot.Service, serviceCount),
	}
	serviceSizes := make([]uint32, serviceCount)

	for i := range bs.Services {
		nameLen, err := r.ReadByte()
		if err != nil {
			return nil, readLen, nil, err
		}
		readLen++

		if nameLen == 0 || nameLen > maxServiceNameLen {
			return nil, readLen, nil, badprogram.Error("service name length out of bounds")
		}

		b := make([]byte, nameLen)
		n, err := io.ReadFull(r, b)
		readLen += n
		if err != nil {
			return nil, readLen, nil, err
		}
		bs.Services[i] = &snapshot.Service{Name: string(b)}

		serviceSizes[i], n, err = binary.Varuint32(r)
		readLen += n
		if err != nil {
			return nil, readLen, nil, err
		}

		// TODO: limit size

		dataSize += int64(serviceSizes[i])
	}

	if int64(readLen)+dataSize > int64(length) {
		return nil, readLen, nil, badprogram.Error("invalid buffer section in wasm module")
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

	return bs, readLen, dataBuf, nil
}
