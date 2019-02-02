// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"syscall"

	"github.com/tsavola/wag/buffer"
	"github.com/tsavola/wag/object"
	"github.com/tsavola/wag/section"
	"github.com/tsavola/wag/wa"
	"github.com/tsavola/wag/wa/opcode"
)

const customStackSectionName = "gate.stack"

func BuildModule(ctx context.Context, storage ModuleStorage, origModule Module, metadata *Metadata, exe *Executable, codeMap *object.CallMap,
) (newModule Module, hash string, err error) {
	origLoad, err := origModule.Open(ctx)
	if err != nil {
		return
	}
	defer origLoad.Close()

	// Dimensions of the memory mapping.  (Text segment is not mapped.)
	var (
		stackOffset   = 0
		globalsOffset = exe.manifest.StackSize
		memoryOffset  = exe.manifest.StackSize + exe.manifest.GlobalsSize
		exeMapSize    = exe.manifest.StackSize + exe.manifest.GlobalsSize + exe.manifest.MaxMemorySize
	)

	exeMap, err := syscall.Mmap(int(exe.file.Fd()), int64(exe.manifest.TextSize), exeMapSize, syscall.PROT_READ, syscall.MAP_PRIVATE)
	if err != nil {
		return
	}
	defer syscall.Munmap(exeMap)

	// Mapped segments.
	var (
		stackMap   = exeMap[stackOffset:globalsOffset]
		globalsMap = exeMap[globalsOffset:memoryOffset]
		memoryMap  = exeMap[memoryOffset:]
	)

	stackUnused, memorySize, textAddr, ok := checkStack(stackMap, len(stackMap))
	if !ok {
		err = ErrBadTermination
		return
	}
	if int(memorySize) > len(memoryMap) {
		err = ErrBadTermination
		return
	}

	// Data without unused regions or padding.
	var (
		stackData   = stackMap[stackUnused:]
		globalsData = globalsMap[len(globalsMap)-len(metadata.GlobalTypes)*8:]
		memoryData  = memoryMap[:memorySize]
	)

	var (
		globalsSection = buildGlobalSection(metadata.GlobalTypes, globalsData)
		dataHeader     = buildDataSectionHeader(len(memoryData))
		stackHeader    = buildStackSectionHeader(len(stackData))
	)

	var (
		b      = buffer.NewDynamic(nil) // TODO: calculate size in advance for accounting
		r      = origLoad.ReaderAt()
		offset int64
	)

	// Copy until global section and advance offset beyond it.

	offset, err = copyAtUntilSkip(b, r, offset, metadata.SectionRanges[section.Global])
	if err != nil {
		return
	}

	// Write new global section.

	copy(b.Extend(len(globalsSection)), globalsSection)

	// If there is a start section, copy sections between global and start, and
	// advance offset beyond it.  Otherwise the next copy will take care of it.

	if metadata.SectionRanges[section.Start].Length > 0 {
		offset, err = copyAtUntilSkip(b, r, offset, metadata.SectionRanges[section.Start])
		if err != nil {
			return
		}
	}

	// Copy sections between global/start and data, and advance offset beyond
	// data.

	offset, err = copyAtUntilSkip(b, r, offset, metadata.SectionRanges[section.Data])
	if err != nil {
		return
	}

	// Write new data section.

	copy(b.Extend(len(dataHeader)), dataHeader)
	copy(b.Extend(len(memoryData)), memoryData)

	// Write (custom) stack section.

	copy(b.Extend(len(stackHeader)), stackHeader)

	err = exportStack(b.Extend(len(stackData)), stackData, textAddr, codeMap)
	if err != nil {
		return
	}

	// Copy remaining (custom) sections.
	// TODO: skip old stack section

	if n := origLoad.Length - offset; n > 0 {
		_, err = r.ReadAt(b.Extend(int(n)), offset)
		if err != nil {
			return
		}
	}

	h := sha512.New384()
	h.Write(b.Bytes())
	hash = base64.URLEncoding.EncodeToString(h.Sum(nil))

	newStore, err := storage.CreateModule(ctx, len(b.Bytes()))
	if err != nil {
		return
	}
	defer newStore.Close()

	_, err = newStore.Write(b.Bytes())
	if err != nil {
		return
	}

	newModule, err = newStore.Module(hash)
	if err != nil {
		return
	}

	return
}

func buildGlobalSection(types []wa.GlobalType, segment []byte) []byte {
	const (
		// Section id, payload length, item count.
		maxHeaderSize = 1 + binary.MaxVarintLen32 + binary.MaxVarintLen32

		// Type, mutable flag, const op, const value, end op.
		maxItemSize = 1 + 1 + 1 + binary.MaxVarintLen64 + 1
	)

	buf := make([]byte, maxHeaderSize+len(types)*maxItemSize)

	// Items:
	itemsSize := putGlobals(buf[maxHeaderSize:], types, segment)

	// Header:
	countSize := putVaruint32Before(buf, maxHeaderSize, uint32(len(types)))
	payloadLen := countSize + itemsSize
	payloadLenSize := putVaruint32Before(buf, maxHeaderSize-countSize, uint32(payloadLen))
	buf[maxHeaderSize-countSize-payloadLenSize-1] = byte(section.Global)

	return buf[maxHeaderSize-countSize-payloadLenSize-1 : maxHeaderSize+itemsSize]
}

func putGlobals(target []byte, types []wa.GlobalType, segment []byte) (totalSize int) {
	for _, t := range types {
		value := binary.LittleEndian.Uint64(segment)
		segment = segment[8:]

		encoded := t.Encode()
		n := copy(target, encoded[:])
		target = target[n:]
		totalSize += n

		switch t.Type() {
		case wa.I32:
			target[0] = byte(opcode.I32Const)
			n = 1 + binary.PutVarint(target[1:], int64(int32(uint32(value))))

		case wa.I64:
			target[0] = byte(opcode.I64Const)
			n = 1 + binary.PutVarint(target[1:], int64(value))

		case wa.F32:
			target[0] = byte(opcode.F32Const)
			binary.LittleEndian.PutUint32(target[1:], uint32(value))
			n = 1 + 4

		case wa.F64:
			target[0] = byte(opcode.F64Const)
			binary.LittleEndian.PutUint64(target[1:], value)
			n = 1 + 8

		default:
			panic(t)
		}
		target = target[n:]
		totalSize += n

		target[0] = byte(opcode.End)
		target = target[1:]
		totalSize += 1
	}

	return
}

func buildDataSectionHeader(memorySize int) []byte {
	const (
		// Section id, payload length.
		maxSectionFrameSize = 1 + binary.MaxVarintLen32

		// Count, memory index, init expression, size.
		maxSegmentHeaderSize = 1 + 1 + 3 + binary.MaxVarintLen32
	)

	buf := make([]byte, maxSectionFrameSize+maxSegmentHeaderSize)

	segment := buf[maxSectionFrameSize:]
	segment[0] = 1 // Count
	segment[1] = 0 // Memory index
	segment[2] = byte(opcode.I32Const)
	segment[3] = 0 // Offset
	segment[4] = byte(opcode.End)
	segmentHeaderSize := 5 + binary.PutUvarint(segment[5:], uint64(memorySize))

	payloadLen := segmentHeaderSize + memorySize
	payloadLenSize := putVaruint32Before(buf, maxSectionFrameSize, uint32(payloadLen))
	buf[maxSectionFrameSize-payloadLenSize-1] = byte(section.Data)

	return buf[maxSectionFrameSize-payloadLenSize-1 : maxSectionFrameSize+segmentHeaderSize]
}

func buildStackSectionHeader(stackSize int) []byte {
	// Section id, payload length.
	const maxSectionFrameSize = 1 + binary.MaxVarintLen32

	// Name length, name string.
	var customHeaderSize = 1 + len(customStackSectionName)

	buf := make([]byte, maxSectionFrameSize+customHeaderSize)
	buf[0] = byte(section.Custom)
	payloadLenSize := binary.PutUvarint(buf[1:], uint64(customHeaderSize+stackSize))
	buf[1+payloadLenSize] = byte(len(customStackSectionName))
	copy(buf[1+payloadLenSize+1:], customStackSectionName)

	return buf[:1+payloadLenSize+1+len(customStackSectionName)]
}

func putVaruint32Before(target []byte, offset int, x uint32) (n int) {
	var temp [binary.MaxVarintLen32]byte
	n = binary.PutUvarint(temp[:], uint64(x))
	copy(target[offset-n:], temp[:n])
	return
}

func copyAtUntilSkip(b *buffer.Dynamic, r io.ReaderAt, oldOffset int64, section section.ByteRange,
) (newOffset int64, err error) {
	if n := section.Offset - oldOffset; n > 0 {
		_, err = r.ReadAt(b.Extend(int(n)), oldOffset)
		if err != nil {
			return
		}
	}

	newOffset = section.Offset + section.Length
	return
}

// exportStack from native source buffer to portable target buffer.
func exportStack(portable, native []byte, textAddr uint64, codeMap *object.CallMap) (err error) {
	if n := len(native); n == 0 || n&7 != 0 {
		err = fmt.Errorf("invalid stack size %d", n)
		return
	}

	for len(native) > 0 {
		absRetAddr := binary.LittleEndian.Uint64(native)

		retAddr := absRetAddr - textAddr
		if retAddr > math.MaxUint32 {
			err = fmt.Errorf("return address 0x%x is not in text section", absRetAddr)
			return
		}

		_, callIndex, _, stackOffset, initial, ok := codeMap.FindAddr(uint32(retAddr))
		if !ok {
			err = fmt.Errorf("call instruction not found for return address 0x%x", retAddr)
			return
		}

		binary.LittleEndian.PutUint64(portable, uint64(callIndex))

		if initial {
			if stackOffset != 8 {
				err = fmt.Errorf("initial function call site 0x%x has inconsistent stack offset %d", retAddr, stackOffset)
				return
			}

			copy(portable[8:], native[8:])
			return
		}

		if stackOffset == 0 || stackOffset&7 != 0 {
			err = fmt.Errorf("invalid stack offset %d", stackOffset)
			return
		}

		copy(portable[8:stackOffset], native[8:stackOffset])

		native = native[stackOffset:]
		portable = portable[stackOffset:]
	}

	err = errors.New("ran out of stack before initial call")
	return
}
