// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"crypto/sha512"
	"encoding/base64"
	"encoding/binary"
	"io"
	"syscall"

	"github.com/tsavola/gate/image/wasm"
	internal "github.com/tsavola/gate/internal/executable"
	"github.com/tsavola/gate/internal/manifest"
	"github.com/tsavola/wag/object/abi"
	"github.com/tsavola/wag/section"
	"github.com/tsavola/wag/wa"
	"github.com/tsavola/wag/wa/opcode"
)

const wasmModuleHeaderSize = 8

func Snapshot(newBack LocalStorage, oldArc LocalArchive, exe *Executable, suspended bool,
) (arcKey string, newArc LocalArchive, err error) {
	// Old archive.
	var (
		arcMan    = oldArc.Manifest()
		oldRanges = arcMan.Sections
		oldFile   = oldArc.file()
	)

	objectMap, err := oldArc.ObjectMap()
	if err != nil {
		return
	}

	// Executable file.
	var (
		exeTextOffset    = int64(0)
		exeStackOffset   = exeTextOffset + alignSize64(int64(exe.Man.TextSize))
		exeGlobalsOffset = exeStackOffset + int64(exe.Man.StackSize)
		exeMemoryOffset  = exeGlobalsOffset + alignSize64(int64(exe.Man.GlobalsSize))
	)

	// Memory mapping dimensions.  (Text and memory are not mapped.)
	var (
		exeMapStackOffset   = 0
		exeMapGlobalsOffset = exeMapStackOffset + int(exe.Man.StackSize)
		exeMapLen           = exeMapGlobalsOffset + alignSize(int(exe.Man.GlobalsSize))
	)

	exeMap, err := syscall.Mmap(int(exe.file.Fd()), alignSize64(int64(exe.Man.TextSize)), exeMapLen, syscall.PROT_READ, syscall.MAP_PRIVATE)
	if err != nil {
		return
	}
	defer syscall.Munmap(exeMap)

	// Mapped segments.
	var (
		exeStackMap   = exeMap[:exeMapGlobalsOffset]
		exeGlobalsMap = exeMap[exeMapGlobalsOffset:]
	)

	exeStackUnused, memorySize, ok := checkStack(exeStackMap, len(exeStackMap))
	if !ok {
		err = ErrBadTermination
		return
	}
	if memorySize > exe.Man.MemorySizeLimit {
		err = ErrBadTermination
		return
	}

	// Stack, globals and memory contents without unused regions or padding.
	var (
		exeStackData []byte
		stackUsage   int
		globalsData  = exeGlobalsMap[len(exeGlobalsMap)-len(arcMan.GlobalTypes)*8:]
		newTextAddr  uint64
	)

	if exeStackUnused != 0 {
		exeStackData = exeStackMap[exeStackUnused:]
		stackUsage = len(exeStackData)
		newTextAddr = exe.Man.TextAddr
	} else {
		stackUsage = 16 // Return address and entry function argument.
	}

	// New module sections.
	// TODO: align section contents to facilitate reflinking?
	// TODO: stitch module together during download?
	var (
		newRanges = make([]manifest.ByteRange, section.Data+1)
	)

	off := int64(wasmModuleHeaderSize)
	off = mapOldSection(off, newRanges, oldRanges, section.Type)
	off = mapOldSection(off, newRanges, oldRanges, section.Import)
	off = mapOldSection(off, newRanges, oldRanges, section.Function)
	off = mapOldSection(off, newRanges, oldRanges, section.Table)

	memorySection := makeMemorySection(memorySize) // TODO: maximum value
	off = mapNewSection(off, newRanges, len(memorySection), section.Memory)

	globalSection := makeGlobalSection(arcMan.GlobalTypes, globalsData)
	off = mapNewSection(off, newRanges, len(globalSection), section.Global)

	off = mapOldSection(off, newRanges, oldRanges, section.Export)
	off = mapOldSection(off, newRanges, oldRanges, section.Element)
	off = mapOldSection(off, newRanges, oldRanges, section.Code)

	stackHeader := makeStackSectionHeader(stackUsage)
	stackSectionSize := len(stackHeader) + stackUsage
	stackSectionOffset := off
	off += int64(stackSectionSize)

	dataHeader := makeDataSectionHeader(int(memorySize))
	dataSectionSize := len(dataHeader) + int(memorySize)
	off = mapNewSection(off, newRanges, dataSectionSize, section.Data)

	// New module size.
	newModuleSize := arcMan.ModuleSize
	newModuleSize -= arcMan.Sections[section.Memory].Length
	newModuleSize -= arcMan.Sections[section.Global].Length
	newModuleSize -= arcMan.Sections[section.Start].Length
	newModuleSize -= arcMan.StackSection.Length
	newModuleSize -= arcMan.Sections[section.Data].Length
	newModuleSize += int64(len(memorySection))
	newModuleSize += int64(len(globalSection))
	newModuleSize += int64(stackSectionSize)
	newModuleSize += int64(dataSectionSize)

	// New archive file.

	newFile, err := newBack.newArchiveFile()
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			newFile.Close()
		}
	}()

	// Copy module header and sections up to and including table section.

	copyLen := int(wasmModuleHeaderSize)
	for i := section.Table; i >= section.Type; i-- {
		if newRanges[i].Offset != 0 {
			copyLen = int(newRanges[i].Offset + newRanges[i].Length)
			break
		}
	}

	newOff := int64(arcModuleOffset)
	oldOff := int64(arcModuleOffset)
	err = copyFileRange(oldFile.Fd(), &oldOff, newFile.Fd(), &newOff, copyLen)
	if err != nil {
		return
	}

	// Write new memory and global section, and skip old ones.

	memoryGlobalSections := append(memorySection, globalSection...)

	n, err := newFile.WriteAt(memoryGlobalSections, newOff)
	if err != nil {
		return
	}
	newOff += int64(n)
	oldOff += oldRanges[section.Memory].Length + oldRanges[section.Global].Length

	// If there is a start section, copy export section separately, and skip
	// start section.

	nextSection := section.Export

	if oldRanges[section.Start].Length > 0 {
		copyLen = int(oldRanges[section.Export].Length)

		err = copyFileRange(oldFile.Fd(), &oldOff, newFile.Fd(), &newOff, copyLen)
		if err != nil {
			return
		}

		oldOff += oldRanges[section.Start].Length
		nextSection = section.Element
	}

	// Copy sections up to and including code section.

	copyLen = 0
	for _, s := range oldRanges[nextSection : section.Code+1] {
		if s.Length > 0 {
			copyLen += int(s.Length)
		}
	}

	err = copyFileRange(oldFile.Fd(), &oldOff, newFile.Fd(), &newOff, copyLen)
	if err != nil {
		return
	}

	// Write new stack section, and skip old one.

	newStackSection := make([]byte, len(stackHeader)+stackUsage)
	copy(newStackSection, stackHeader)

	newStackData := newStackSection[len(stackHeader):]
	if exeStackData != nil {
		err = exportStack(newStackData, exeStackData, exe.Man.TextAddr, objectMap) // TODO: in-place?
		if err != nil {
			return
		}
	} else {
		// Synthesize portable stack, suspended at virtual call site at index 0.
		binary.LittleEndian.PutUint32(newStackData[8:], exe.entryIndex)
	}

	n, err = newFile.WriteAt(newStackSection, newOff)
	if err != nil {
		return
	}
	newOff += int64(n)
	oldOff += arcMan.StackSection.Length

	// Copy new data section from executable, and skip old archived one.

	n, err = newFile.WriteAt(dataHeader, newOff)
	if err != nil {
		return
	}
	newOff += int64(n)

	exeOff := exeMemoryOffset
	err = copyFileRange(exe.file.Fd(), &exeOff, newFile.Fd(), &newOff, int(memorySize))
	if err != nil {
		return
	}
	oldOff += oldRanges[section.Data].Length

	// Copy remaining (custom) sections.

	copyLen = int(arcMan.ModuleSize - oldOff)

	err = copyFileRange(oldFile.Fd(), &oldOff, newFile.Fd(), &newOff, copyLen)
	if err != nil {
		return
	}

	// Module key.

	h := sha512.New384()

	_, err = io.Copy(h, io.NewSectionReader(newFile, arcModuleOffset, newModuleSize))
	if err != nil {
		return
	}

	arcKey = base64.URLEncoding.EncodeToString(h.Sum(nil))

	// Copy object map from archive.
	var (
		objectMapSize = int(arcMan.CallSitesSize) + int(arcMan.FuncAddrsSize)
	)

	err = copyFileRange(oldFile.Fd(), &oldOff, newFile.Fd(), &newOff, objectMapSize)
	if err != nil {
		return
	}

	// Copy text from archive.  (Archives likely share the same backend.)

	newOff = alignSize64(newOff)
	oldOff = alignSize64(oldOff)
	err = copyFileRange(oldFile.Fd(), &oldOff, newFile.Fd(), &newOff, alignSize(int(exe.Man.TextSize)))
	if err != nil {
		return
	}

	// Copy stack, globals and memory from executable (again).
	var (
		newStackSize = alignSize(stackUsage)
	)

	newOff = alignSize64(newOff)
	exeOff = alignSize64(exeStackOffset)

	if exeStackData != nil {
		copyLen = alignSize(stackUsage)

		newOff += int64(newStackSize) - int64(copyLen)
		exeOff += int64(exe.Man.StackSize) - int64(copyLen)
		err = copyFileRange(exe.file.Fd(), &exeOff, newFile.Fd(), &newOff, copyLen)
		if err != nil {
			return
		}
	} else {
		// Replace portable index with native address.
		binary.LittleEndian.PutUint32(newStackData[8:], exe.entryAddr)

		newOff += int64(newStackSize) - int64(stackUsage)
		n, err = newFile.WriteAt(newStackData, newOff)
		if err != nil {
			return
		}
		newOff += int64(n)
		exeOff += int64(exe.Man.StackSize)
	}

	copyLen = alignSize(int(exe.Man.GlobalsSize)) + int(memorySize)

	err = copyFileRange(exe.file.Fd(), &exeOff, newFile.Fd(), &newOff, copyLen)
	if err != nil {
		return
	}

	// New archive manifest.

	arcMan.ModuleSize = newModuleSize
	arcMan.Sections = newRanges
	arcMan.StackSection = manifest.ByteRange{
		Offset: stackSectionOffset,
		Length: int64(stackSectionSize),
	}
	arcMan.Exe.TextAddr = newTextAddr
	arcMan.Exe.StackSize = uint32(newStackSize)
	arcMan.Exe.StackUsage = uint32(stackUsage)
	arcMan.Exe.MemoryDataSize = memorySize
	arcMan.Exe.MemorySize = memorySize
	arcMan.Exe.InitRoutine = abi.TextAddrResume

	// Archive it.

	newArc, err = newBack.give(arcKey, arcMan, internal.NewFileRef(newFile), objectMap)
	if err != nil {
		return
	}

	return
}

func makeMemorySection(currentMemorySize uint32) []byte {
	buf := make([]byte, 4+binary.MaxVarintLen32)

	n := binary.PutUvarint(buf[4:], uint64(currentMemorySize>>wa.PageBits)) // Initial value
	buf[3] = 0                                                              // Maximum flag
	buf[2] = 1                                                              // Item count
	buf[1] = byte(2 + n)                                                    // Payload length
	buf[0] = byte(section.Memory)                                           // Section id

	return buf[:4+n]
}

func makeGlobalSection(globalTypes []byte, segment []byte) []byte {
	const (
		// Section id, payload length, item count.
		maxHeaderSize = 1 + binary.MaxVarintLen32 + binary.MaxVarintLen32

		// Type, mutable flag, const op, const value, end op.
		maxItemSize = 1 + 1 + 1 + binary.MaxVarintLen64 + 1
	)

	buf := make([]byte, maxHeaderSize+len(globalTypes)*maxItemSize)

	// Items:
	itemsSize := putGlobals(buf[maxHeaderSize:], globalTypes, segment)

	// Header:
	countSize := putVaruint32Before(buf, maxHeaderSize, uint32(len(globalTypes)))
	payloadLen := countSize + itemsSize
	payloadLenSize := putVaruint32Before(buf, maxHeaderSize-countSize, uint32(payloadLen))
	buf[maxHeaderSize-countSize-payloadLenSize-1] = byte(section.Global)

	return buf[maxHeaderSize-countSize-payloadLenSize-1 : maxHeaderSize+itemsSize]
}

func putGlobals(target []byte, globalTypes []byte, segment []byte) (totalSize int) {
	for _, b := range globalTypes {
		t := wa.GlobalType(b)

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
		totalSize++
	}

	return
}

func makeStackSectionHeader(stackSize int) []byte {
	// Section id, payload length.
	const maxSectionFrameSize = 1 + binary.MaxVarintLen32

	// Name length, name string.
	var customHeaderSize = 1 + len(wasm.StackSectionName)

	buf := make([]byte, maxSectionFrameSize+customHeaderSize)
	buf[0] = byte(section.Custom)
	payloadLenSize := binary.PutUvarint(buf[1:], uint64(customHeaderSize+stackSize))
	buf[1+payloadLenSize] = byte(len(wasm.StackSectionName))
	copy(buf[1+payloadLenSize+1:], wasm.StackSectionName)

	return buf[:1+payloadLenSize+1+len(wasm.StackSectionName)]
}

func makeDataSectionHeader(memorySize int) []byte {
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

func putVaruint32Before(target []byte, offset int, x uint32) (n int) {
	var temp [binary.MaxVarintLen32]byte
	n = binary.PutUvarint(temp[:], uint64(x))
	copy(target[offset-n:], temp[:n])
	return
}

func mapOldSection(offset int64, dest, src []manifest.ByteRange, i section.ID) int64 {
	if src[i].Length > 0 {
		dest[i] = manifest.ByteRange{
			Offset: offset,
			Length: src[i].Length,
		}
		offset += src[i].Length
	}
	return offset
}

func mapNewSection(offset int64, dest []manifest.ByteRange, length int, i section.ID) int64 {
	dest[i] = manifest.ByteRange{
		Offset: offset,
		Length: int64(length),
	}
	return offset + int64(length)
}
