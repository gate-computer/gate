// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"encoding/binary"
	"syscall"

	"gate.computer/gate/internal/file"
	"gate.computer/gate/internal/manifest"
	"gate.computer/gate/internal/varint"
	"gate.computer/gate/snapshot"
	"gate.computer/gate/snapshot/wasm"
	"gate.computer/wag/section"
	"gate.computer/wag/wa"
	"gate.computer/wag/wa/opcode"
)

const wasmModuleHeaderSize = 8
const snapshotVersion = 0

func Snapshot(oldProg *Program, inst *Instance, buffers snapshot.Buffers, suspended bool) (*Program, error) {
	// Instance file.
	var (
		instStackOffset   = int64(0)
		instGlobalsOffset = instStackOffset + int64(inst.man.StackSize)
		instMemoryOffset  = instGlobalsOffset + alignPageOffset32(inst.man.GlobalsSize)
	)

	var (
		stackUsage   int
		stackMapSize int
		newTextAddr  uint64
	)
	if suspended || inst.Final() {
		if inst.man.StackUsage != 0 {
			stackUsage = int(inst.man.StackUsage)
			stackMapSize = alignPageSize(stackUsage)
			newTextAddr = inst.man.TextAddr
		} else if suspended {
			// Resume at virtual call site at beginning of enter routine.
			// Stack data is synthesized later.
			stackUsage = initStackSize
		}
	}

	// TODO: reading might be faster.  exportStack could work in-place.
	instMapOffset := instGlobalsOffset - int64(stackMapSize)
	instMap, err := mmap(inst.file.FD(), instMapOffset, int(instMemoryOffset-instMapOffset), syscall.PROT_READ, syscall.MAP_PRIVATE)
	if err != nil {
		return nil, err
	}
	defer mustMunmap(instMap)

	var (
		instGlobalsMapping = instMap[stackMapSize:]
		instGlobalsData    = instGlobalsMapping[len(instGlobalsMapping)-len(oldProg.man.GlobalTypes)*8:]
		instStackData      []byte
	)
	if stackMapSize != 0 {
		instStackData = instMap[stackMapSize-stackUsage : stackMapSize]
	}

	// New module sections.
	oldRanges := oldProg.man.Sections
	newRanges := make([]*manifest.ByteRange, section.Data+1)

	off := int64(wasmModuleHeaderSize)
	off = mapOldSection(off, newRanges, oldRanges, section.Type)
	off = mapOldSection(off, newRanges, oldRanges, section.Import)
	off = mapOldSection(off, newRanges, oldRanges, section.Function)
	off = mapOldSection(off, newRanges, oldRanges, section.Table)

	memorySection := makeMemorySection(inst.man.MemorySize, oldProg.man.MemorySizeLimit)
	off = mapNewSection(off, newRanges, len(memorySection), section.Memory)

	globalSection := makeGlobalSection(oldProg.man.GlobalTypes, instGlobalsData)
	off = mapNewSection(off, newRanges, len(globalSection), section.Global)

	snapshotSection := makeSnapshotSection(inst.man.Snapshot)
	snapshotSectionOffset := off
	off += int64(len(snapshotSection))

	var (
		exportSectionWrap      *manifest.ByteRange
		exportSectionWrapFrame []byte
	)
	if suspended || inst.Final() {
		if oldRanges[section.Export].Length != 0 {
			if oldLen := oldProg.man.ExportSectionWrap.GetLength(); oldLen != 0 {
				exportSectionWrap = &manifest.ByteRange{
					Offset: off,
					Length: oldLen,
				}
			} else {
				exportSectionWrapFrame = makeExportSectionWrapFrame(oldRanges[section.Export].Length)
				exportSectionWrap = &manifest.ByteRange{
					Offset: off,
					Length: int64(len(exportSectionWrapFrame)) + oldRanges[section.Export].Length,
				}
			}
			off += exportSectionWrap.Length
			newRanges[section.Export] = &manifest.ByteRange{
				Offset: off - oldRanges[section.Export].Length,
				Length: oldRanges[section.Export].Length,
			}
		}
	} else {
		off = mapOldSection(off, newRanges, oldRanges, section.Export)
	}

	off = mapOldSection(off, newRanges, oldRanges, section.Element)
	off = mapOldSection(off, newRanges, oldRanges, section.Code)

	bufferHeader, bufferSectionSize := makeBufferSectionHeader(buffers)
	bufferSectionOffset := off
	off += bufferSectionSize

	var (
		stackHeader        []byte
		stackSectionSize   int
		stackSectionOffset int64
	)
	if stackUsage != 0 {
		stackHeader = makeStackSectionHeader(stackUsage)
		stackSectionSize = len(stackHeader) + stackUsage
		stackSectionOffset = off
		off += int64(stackSectionSize)
	}

	dataHeader := makeDataSectionHeader(int(inst.man.MemorySize))
	dataSectionSize := len(dataHeader) + int(inst.man.MemorySize)
	off = mapNewSection(off, newRanges, dataSectionSize, section.Data)

	// New module size.
	newModuleSize := oldProg.man.ModuleSize

	newModuleSize -= oldRanges[section.Memory].Length
	newModuleSize -= oldRanges[section.Global].Length
	newModuleSize -= oldProg.man.SnapshotSection.GetLength()
	newModuleSize -= oldProg.man.ExportSectionWrap.GetLength()
	if oldProg.man.ExportSectionWrap.GetLength() == 0 {
		newModuleSize -= oldRanges[section.Export].Length
	}
	newModuleSize -= oldRanges[section.Start].Length
	newModuleSize -= oldProg.man.BufferSection.GetLength()
	newModuleSize -= oldProg.man.StackSection.GetLength()
	newModuleSize -= oldRanges[section.Data].Length

	newModuleSize += int64(len(memorySection))
	newModuleSize += int64(len(globalSection))
	newModuleSize += int64(len(snapshotSection))
	newModuleSize += exportSectionWrap.GetLength()
	if exportSectionWrap.GetLength() == 0 {
		newModuleSize += newRanges[section.Export].Length
	}
	newModuleSize += bufferSectionSize
	newModuleSize += int64(stackSectionSize)
	newModuleSize += int64(dataSectionSize)

	// New program file.
	newFile, err := oldProg.storage.newProgramFile()
	if err != nil {
		return nil, err
	}
	defer func() {
		if newFile != nil {
			newFile.Close()
		}
	}()

	// Copy module header and sections up to and including table section.
	copyLen := int(wasmModuleHeaderSize)
	for i := section.Table; i >= section.Type; i-- {
		if newRanges[i].Length != 0 {
			copyLen = int(newRanges[i].Offset + newRanges[i].Length)
			break
		}
	}

	newOff := progModuleOffset
	oldOff := progModuleOffset
	if err := copyFileRange(oldProg.file, &oldOff, newFile, &newOff, copyLen); err != nil {
		return nil, err
	}

	// Write new memory and global section, and skip old ones.
	n, err := newFile.WriteAt(append(memorySection, globalSection...), newOff)
	if err != nil {
		return nil, err
	}
	newOff += int64(n)
	oldOff += oldRanges[section.Memory].Length + oldRanges[section.Global].Length

	// Write new snapshot section, and skip old one.
	n, err = newFile.WriteAt(snapshotSection, newOff)
	if err != nil {
		return nil, err
	}
	newOff += int64(n)
	oldOff += oldProg.man.SnapshotSection.GetLength()

	// Copy export section, possibly writing or skipping wrapper.
	copyLen = int(oldRanges[section.Export].Length)
	if exportSectionWrap.GetLength() != 0 {
		if oldLen := oldProg.man.ExportSectionWrap.GetLength(); oldLen != 0 {
			copyLen = int(oldLen)
		} else {
			n, err = newFile.WriteAt(exportSectionWrapFrame, newOff)
			if err != nil {
				return nil, err
			}
			newOff += int64(n)
		}
	} else {
		if oldLen := oldProg.man.ExportSectionWrap.GetLength(); oldLen != 0 {
			oldOff += oldLen - oldRanges[section.Export].Length
		}
	}
	if err := copyFileRange(oldProg.file, &oldOff, newFile, &newOff, copyLen); err != nil {
		return nil, err
	}

	// Skip start section.
	oldOff += oldRanges[section.Start].Length

	// Copy element and code sections.
	copyLen = int(oldRanges[section.Element].Length + oldRanges[section.Code].Length)
	if err := copyFileRange(oldProg.file, &oldOff, newFile, &newOff, copyLen); err != nil {
		return nil, err
	}

	// Write new buffer section, and skip old one.
	if bufferSectionSize > 0 {
		n, err = newFile.WriteAt(bufferHeader, newOff)
		if err != nil {
			return nil, err
		}
		newOff += int64(n)

		n, err = writeBufferSectionDataAt(newFile, buffers, newOff)
		if err != nil {
			return nil, err
		}
		newOff += int64(n)
	}
	oldOff += oldProg.man.BufferSection.GetLength()

	// Write new stack section, and skip old one.
	if stackSectionSize > 0 {
		newStackSection := make([]byte, stackSectionSize)
		copy(newStackSection, stackHeader)

		newStack := newStackSection[len(stackHeader):]

		if instStackData != nil {
			if err := exportStack(newStack, instStackData, inst.man.TextAddr, &oldProg.Map); err != nil {
				return nil, err
			}
		} else {
			putInitStack(newStack, inst.man.StartFunc, inst.man.EntryFunc)
		}

		n, err = newFile.WriteAt(newStackSection, newOff)
		if err != nil {
			return nil, err
		}
		newOff += int64(n)
	}
	oldOff += oldProg.man.StackSection.GetLength()

	// Copy new data section from instance, and skip old one.
	n, err = newFile.WriteAt(dataHeader, newOff)
	if err != nil {
		return nil, err
	}
	newOff += int64(n)

	instOff := instMemoryOffset
	if err := copyFileRange(inst.file, &instOff, newFile, &newOff, int(inst.man.MemorySize)); err != nil {
		return nil, err
	}
	oldOff += oldRanges[section.Data].Length

	// Copy remaining (custom) sections.
	copyLen = int(oldProg.man.ModuleSize - (oldOff - progModuleOffset))
	if err := copyFileRange(oldProg.file, &oldOff, newFile, &newOff, copyLen); err != nil {
		return nil, err
	}

	// Copy object map from program.
	newOff = align8(newOff)
	oldOff = align8(oldOff)
	if err := copyFileRange(oldProg.file, &oldOff, newFile, &newOff, int(oldProg.man.CallSitesSize)+int(oldProg.man.FuncAddrsSize)); err != nil {
		return nil, err
	}

	// Copy text from program.
	newOff = progTextOffset
	oldOff = progTextOffset
	if err := copyFileRange(oldProg.file, &oldOff, newFile, &newOff, alignPageSize32(oldProg.man.TextSize)); err != nil {
		return nil, err
	}

	// Copy stack from instance (again).
	if instStackData != nil {
		copyLen := alignPageSize(stackUsage)
		newOff = progGlobalsOffset - int64(copyLen)
		instOff := instGlobalsOffset - int64(copyLen)
		if err := copyFileRange(inst.file, &instOff, newFile, &newOff, copyLen); err != nil {
			return nil, err
		}
	}

	// Copy globals and memory from instance (again).
	newOff = progGlobalsOffset
	instOff = instGlobalsOffset
	if err := copyFileRange(inst.file, &instOff, newFile, &newOff, alignPageSize32(inst.man.GlobalsSize)+int(inst.man.MemorySize)); err != nil {
		return nil, err
	}

	newProg := &Program{
		Map:     oldProg.Map,
		storage: oldProg.storage,
		man: &manifest.Program{
			LibraryChecksum: oldProg.man.LibraryChecksum,
			TextRevision:    oldProg.man.TextRevision,
			TextAddr:        newTextAddr,
			TextSize:        oldProg.man.TextSize,
			StackUsage:      uint32(stackUsage),
			GlobalsSize:     oldProg.man.GlobalsSize,
			MemorySize:      inst.man.MemorySize,
			MemorySizeLimit: oldProg.man.MemorySizeLimit,
			MemoryDataSize:  inst.man.MemorySize,
			ModuleSize:      newModuleSize,
			Sections:        newRanges,
			SnapshotSection: &manifest.ByteRange{
				Offset: snapshotSectionOffset,
				Length: int64(len(snapshotSection)),
			},
			ExportSectionWrap: exportSectionWrap,
			BufferSection: &manifest.ByteRange{
				Offset: bufferSectionOffset,
				Length: bufferSectionSize,
			},
			BufferSectionHeaderLength: int64(len(bufferHeader)),
			StackSection: &manifest.ByteRange{
				Offset: stackSectionOffset,
				Length: int64(stackSectionSize),
			},
			GlobalTypes:   oldProg.man.GlobalTypes,
			StartFunc:     oldProg.man.StartFunc,
			EntryIndexes:  oldProg.man.EntryIndexes,
			EntryAddrs:    oldProg.man.EntryAddrs,
			CallSitesSize: oldProg.man.CallSitesSize,
			FuncAddrsSize: oldProg.man.FuncAddrsSize,
			Random:        oldProg.man.Random,
			Snapshot:      oldProg.man.Snapshot.Clone(),
		},
		file: newFile,
	}
	newFile = nil
	return newProg, nil
}

func makeMemorySection(memorySize uint32, memorySizeLimit int64) []byte {
	b := make([]byte, 4+binary.MaxVarintLen32*2)
	n := len(b)

	var maxFlag byte
	if memorySizeLimit >= 0 {
		n -= putVaruint32Before(b, n, uint32(memorySizeLimit>>wa.PageBits))
		maxFlag = 1
	}
	n -= putVaruint32Before(b, n, uint32(memorySize>>wa.PageBits))
	n--
	b[n] = maxFlag
	n--
	b[n] = 1 // Item count.

	payloadLen := len(b) - n

	n--
	b[n] = byte(payloadLen)
	n--
	b[n] = byte(section.Memory) // Section id.

	return b[n:]
}

func makeGlobalSection(globalTypes []byte, segment []byte) []byte {
	if len(globalTypes) == 0 {
		return nil
	}

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
			n = 1 + putVarint(target[1:], int64(int32(uint32(value))))

		case wa.I64:
			target[0] = byte(opcode.I64Const)
			n = 1 + putVarint(target[1:], int64(value))

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

func makeSnapshotSection(snap *manifest.Snapshot) []byte {
	manifest.InflateSnapshot(&snap)

	// Section id, payload length.
	const maxSectionFrameSize = 1 + binary.MaxVarintLen32

	var maxPayloadSize = (0 +
		1 + // Name length
		len(wasm.SectionSnapshot) + // Name string
		1 + // Snapshot version
		binary.MaxVarintLen64 + // Flags
		binary.MaxVarintLen32 + // Trap
		binary.MaxVarintLen32 + // Result
		binary.MaxVarintLen64 + // Monotonic time
		binary.MaxVarintLen32 + // Breakpoint count
		binary.MaxVarintLen64*len(snap.Breakpoints)) // Breakpoint array

	b := make([]byte, maxSectionFrameSize+maxPayloadSize)
	i := maxSectionFrameSize
	b[i] = byte(len(wasm.SectionSnapshot))
	i++
	i += copy(b[i:], wasm.SectionSnapshot)
	b[i] = snapshotVersion
	i++
	i += binary.PutUvarint(b[i:], snap.Flags)
	i += binary.PutUvarint(b[i:], uint64(snap.Trap))
	i += binary.PutUvarint(b[i:], uint64(snap.Result))
	i += binary.PutUvarint(b[i:], snap.MonotonicTime)
	i += binary.PutUvarint(b[i:], uint64(len(snap.Breakpoints)))
	for _, offset := range snap.Breakpoints {
		i += binary.PutUvarint(b[i:], uint64(offset))
	}

	payloadLen := uint32(i - maxSectionFrameSize)
	payloadLenSize := putVaruint32Before(b, maxSectionFrameSize, payloadLen)
	b[maxSectionFrameSize-payloadLenSize-1] = byte(section.Custom)
	return b[maxSectionFrameSize-payloadLenSize-1 : i]
}

func makeExportSectionWrapFrame(exportSectionSize int64) []byte {
	// Section id, payload length.
	const maxSectionFrameSize = 1 + binary.MaxVarintLen32

	// Name length, name string.
	var nameHeaderSize = 1 + len(wasm.SectionExport)

	b := make([]byte, maxSectionFrameSize+nameHeaderSize)
	i := maxSectionFrameSize
	b[i] = byte(len(wasm.SectionExport))
	i++
	i += copy(b[i:], wasm.SectionExport)

	payloadLen := uint32(nameHeaderSize) + uint32(exportSectionSize)
	payloadLenSize := putVaruint32Before(b, maxSectionFrameSize, payloadLen)
	b[maxSectionFrameSize-payloadLenSize-1] = byte(section.Custom)
	return b[maxSectionFrameSize-payloadLenSize-1 : i]
}

func makeBufferSectionHeader(buffers snapshot.Buffers) (header []byte, sectionSize int64) {
	if len(buffers.Services) == 0 && len(buffers.Input) == 0 && len(buffers.Output) == 0 {
		return
	}

	// Section id, payload length.
	const maxSectionFrameSize = 1 + varint.MaxLen

	maxHeaderSize := maxSectionFrameSize
	maxHeaderSize += 1                       // Section name length
	maxHeaderSize += len(wasm.SectionBuffer) // Section name
	maxHeaderSize += varint.MaxLen           // Input data size
	maxHeaderSize += varint.MaxLen           // Output data size
	maxHeaderSize += varint.MaxLen           // Service count

	for _, s := range buffers.Services {
		maxHeaderSize += 1             // Service name length
		maxHeaderSize += len(s.Name)   // Service name
		maxHeaderSize += varint.MaxLen // Service data size
	}

	buf := make([]byte, maxHeaderSize)

	tail := buf[maxSectionFrameSize:]
	tail = putByte(tail, byte(len(wasm.SectionBuffer)))
	tail = putString(tail, wasm.SectionBuffer)
	tail = varint.Put(tail, int32(len(buffers.Input)))
	tail = varint.Put(tail, int32(len(buffers.Output)))
	tail = varint.Put(tail, int32(len(buffers.Services)))

	dataSize := int64(len(buffers.Input)) + int64(len(buffers.Output))

	for _, s := range buffers.Services {
		tail = putByte(tail, byte(len(s.Name)))
		tail = putString(tail, s.Name)
		tail = varint.Put(tail, int32(len(s.Buffer)))

		dataSize += int64(len(s.Buffer))
	}

	payloadLen := uint32(len(buf)-len(tail)-maxSectionFrameSize) + uint32(dataSize)
	payloadLenSize := putVaruint32Before(buf, maxSectionFrameSize, payloadLen)
	buf[maxSectionFrameSize-payloadLenSize-1] = byte(section.Custom)
	buf = buf[maxSectionFrameSize-payloadLenSize-1:]

	header = buf[:len(buf)-len(tail)]
	sectionSize = int64(len(header)) + dataSize
	return
}

func writeBufferSectionDataAt(f *file.File, bs snapshot.Buffers, off int64) (total int, err error) {
	n, err := f.WriteAt(bs.Input, off)
	if err != nil {
		return
	}
	total += n
	off += int64(n)

	n, err = f.WriteAt(bs.Output, off)
	if err != nil {
		return
	}
	total += n
	off += int64(n)

	for _, s := range bs.Services {
		n, err = f.WriteAt(s.Buffer, off)
		if err != nil {
			return
		}
		total += n
		off += int64(n)
	}

	return
}

func makeStackSectionHeader(stackSize int) []byte {
	// Section id, payload length.
	const maxSectionFrameSize = 1 + binary.MaxVarintLen32

	// Name length, name string.
	var customHeaderSize = 1 + len(wasm.SectionStack)

	buf := make([]byte, maxSectionFrameSize+customHeaderSize)
	buf[0] = byte(section.Custom)
	payloadLenSize := binary.PutUvarint(buf[1:], uint64(customHeaderSize+stackSize))
	buf[1+payloadLenSize] = byte(len(wasm.SectionStack))
	copy(buf[1+payloadLenSize+1:], wasm.SectionStack)

	return buf[:1+payloadLenSize+1+len(wasm.SectionStack)]
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

func putByte(dest []byte, x byte) (tail []byte) {
	dest[0] = x
	return dest[1:]
}

func putString(dest []byte, s string) (tail []byte) {
	copy(dest, s)
	return dest[len(s):]
}

func putVaruint32Before(dest []byte, offset int, x uint32) (n int) {
	var temp [binary.MaxVarintLen32]byte
	n = binary.PutUvarint(temp[:], uint64(x))
	copy(dest[offset-n:], temp[:n])
	return
}

func mapOldSection(offset int64, dest, src []*manifest.ByteRange, i section.ID) int64 {
	if src[i].Length != 0 {
		dest[i] = &manifest.ByteRange{
			Offset: offset,
			Length: src[i].Length,
		}
		offset += src[i].Length
	}
	return offset
}

func mapNewSection(offset int64, dest []*manifest.ByteRange, length int, i section.ID) int64 {
	if length != 0 {
		dest[i] = &manifest.ByteRange{
			Offset: offset,
			Length: int64(length),
		}
	}
	return offset + int64(length)
}

func putVarint(dest []byte, x int64) (n int) {
	for {
		n++
		b := byte(x & 0x7f)
		x >>= 7
		if (x == 0 && b&0x40 == 0) || (x == -1 && b&0x40 != 0) {
			dest[0] = b
			return
		} else {
			dest[0] = b | 0x80
		}
		dest = dest[1:]
	}
}
