// Copyright (c) 2024 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"bytes"
	"encoding/binary"
	"errors"
	"unsafe"

	"gate.computer/internal/file"
	"gate.computer/wag/section"
	"gate.computer/wag/wa/opcode"
	"golang.org/x/sys/unix"

	"import.name/pan"
	. "import.name/pan/mustcheck"
)

// mustMakeDataSection splits memory into segments, skipping long ranges of
// zero bytes.  The memory buffer must be equivalent to the file contents at
// the given base offset.  The returned buffer contains a WebAssembly data
// section.
func mustMakeDataSection(f *file.File, offset int64, memory []byte) []byte {
	// Section id, payload size, segment count.
	headSpace := 1 + binary.MaxVarintLen32 + binary.MaxVarintLen32

	buf := bytes.NewBuffer(make([]byte, headSpace))

	count := mustScanDataHoles(buf, f, offset, memory)

	b := buf.Bytes()

	headSpace -= putVaruint32Before(b, headSpace, uint32(count))

	payloadLen := len(b) - headSpace
	headSpace -= putVaruint32Before(b, headSpace, uint32(payloadLen))

	headSpace--
	b[headSpace] = byte(section.Data)

	return b[headSpace:]
}

func mustScanDataHoles(buf *bytes.Buffer, f *file.File, base int64, memory []byte) (count int) {
	fd := Must(unix.FcntlInt(f.Fd(), unix.F_DUPFD_CLOEXEC, 0))
	defer unix.Close(fd)

	space := int64(len(memory))
	data := Must(unix.Seek(fd, base, unix.SEEK_DATA))

	for data-base < space {
		hole, err := unix.Seek(fd, data, unix.SEEK_HOLE)
		if err != nil {
			if errors.Is(err, unix.ENXIO) {
				break
			}
			pan.Panic(err)
		}

		count += scanData(buf, memory, data-base, min(hole-base, space))

		data, err = unix.Seek(fd, hole, unix.SEEK_DATA)
		if err != nil {
			if errors.Is(err, unix.ENXIO) {
				break
			}
			pan.Panic(err)
		}
	}

	return count
}

func scanData(buf *bytes.Buffer, memory []byte, offset, dataEnd int64) (count int) {
	const (
		alignment = 8
		mask      = alignment - 1
	)

	if offset&mask != 0 {
		end := (offset + mask) &^ mask
		end = min(end, dataEnd)
		count += writeDataSegment(buf, memory, offset, end)
		offset = end
	}

	alignEnd := dataEnd &^ mask
	count += unsafeScanData8(buf, memory, offset, alignEnd)
	count += writeDataSegment(buf, memory, alignEnd, dataEnd)
	return
}

func unsafeScanData8(buf *bytes.Buffer, memory []byte, offset, dataEnd int64) (count int) {
	memory8 := unsafe.Slice((*uint64)(unsafe.Pointer(unsafe.SliceData(memory[offset:dataEnd]))), (dataEnd-offset)/8)
	var base int

	for {
		var i int
		var x uint64

		for i, x = range memory8[base:] {
			if x != 0 {
				goto found
			}
		}
		return

	found:
		start := base + i

		var length int
		end := true

		for length, x = range memory8[start:] {
			if x == 0 {
				end = false
				break
			}
		}

		if end {
			count += writeDataSegment(buf, memory, offset+int64(start*8), dataEnd)
			return
		}

		count += writeDataSegment(buf, memory, offset+int64(start*8), offset+int64(start+length)*8)
		base = start + length
	}
}

func writeDataSegment(buf *bytes.Buffer, memory []byte, offset, end int64) int {
	for offset < end && memory[offset] == 0 {
		offset++
	}
	for offset < end && memory[end-1] == 0 {
		end--
	}
	if offset == end {
		return 0
	}

	// Memory index
	buf.WriteByte(0)

	// Offset expression type
	buf.WriteByte(byte(opcode.I32Const))

	// Offset value
	b := make([]byte, binary.MaxVarintLen32)
	n := putVarint(b, offset)
	buf.Write(b[:n])

	buf.WriteByte(byte(opcode.End))

	// Segment size
	n = putVarint(b, end-offset)
	buf.Write(b[:n])

	// Segment data
	buf.Write(memory[offset:end])

	return 1
}
