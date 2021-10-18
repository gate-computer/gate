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
		stackUsage  int
		stackMapLen int
		newTextAddr uint64
	)
	if suspended || inst.Final() {
		if inst.man.StackUsage != 0 {
			stackUsage = int(inst.man.StackUsage)
			stackMapLen = alignPageSize(stackUsage)
			newTextAddr = inst.man.TextAddr
		} else if suspended {
			// Resume at virtual call site at beginning of enter routine.
			// Stack data is synthesized later.
			stackUsage = initStackSize
		}
	}

	// TODO: reading might be faster.  exportStack could work in-place.
	instMapOffset := instGlobalsOffset - int64(stackMapLen)
	instMap, err := mmap(inst.file.FD(), instMapOffset, int(instMemoryOffset-instMapOffset), syscall.PROT_READ, syscall.MAP_PRIVATE)
	if err != nil {
		return nil, err
	}
	defer mustMunmap(instMap)

	var (
		instGlobalsMapping = instMap[stackMapLen:]
		instGlobalsData    = instGlobalsMapping[len(instGlobalsMapping)-len(oldProg.man.GlobalTypes)*8:]
		instStackData      []byte
	)
	if stackMapLen != 0 {
		instStackData = instMap[stackMapLen-stackUsage : stackMapLen]
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
	// TODO: ensure that memorySection size is within bounds
	off = mapNewSection(off, newRanges, uint32(len(memorySection)), section.Memory)

	globalSection := makeGlobalSection(oldProg.man.GlobalTypes, instGlobalsData)
	// TODO: ensure that globalSection size is within bounds
	off = mapNewSection(off, newRanges, uint32(len(globalSection)), section.Global)

	snapshotSection := makeSnapshotSection(inst.man.Snapshot)
	snapshotSectionOffset := off
	off += int64(len(snapshotSection))

	var (
		exportSectionWrap      *manifest.ByteRange
		exportSectionWrapFrame []byte
	)
	if suspended || inst.Final() {
		if oldRanges[section.Export].Size != 0 {
			if oldSize := oldProg.man.ExportSectionWrap.GetSize(); oldSize != 0 {
				exportSectionWrap = &manifest.ByteRange{
					Start: off,
					Size:  oldSize,
				}
			} else {
				exportSectionWrapFrame = makeExportSectionWrapFrame(oldRanges[section.Export].Size)
				exportSectionWrap = &manifest.ByteRange{
					Start: off,
					Size:  uint32(len(exportSectionWrapFrame)) + oldRanges[section.Export].Size,
				}
			}
			off += int64(exportSectionWrap.Size)
			newRanges[section.Export] = &manifest.ByteRange{
				Start: off - int64(oldRanges[section.Export].Size),
				Size:  oldRanges[section.Export].Size,
			}
		}
	} else {
		off = mapOldSection(off, newRanges, oldRanges, section.Export)
	}

	off = mapOldSection(off, newRanges, oldRanges, section.Element)
	off = mapOldSection(off, newRanges, oldRanges, section.Code)

	bufferHeader, bufferSectionSize := makeBufferSectionHeader(buffers)
	bufferSectionOffset := off
	off += int64(bufferSectionSize)

	var (
		stackHeader        []byte
		stackSectionLen    int
		stackSectionOffset int64
	)
	if stackUsage != 0 {
		stackHeader = makeStackSectionHeader(stackUsage)
		stackSectionLen = len(stackHeader) + stackUsage
		stackSectionOffset = off
		off += int64(stackSectionLen)
	}

	dataHeader := makeDataSectionHeader(int(inst.man.MemorySize))
	dataSectionLen := len(dataHeader) + int(inst.man.MemorySize)
	// TODO: check if dataSectionLen is out of bounds
	off = mapNewSection(off, newRanges, uint32(dataSectionLen), section.Data)

	// New module size.
	newModuleSize := oldProg.man.ModuleSize

	newModuleSize -= int64(oldRanges[section.Memory].Size)
	newModuleSize -= int64(oldRanges[section.Global].Size)
	newModuleSize -= int64(oldProg.man.SnapshotSection.GetSize())
	newModuleSize -= int64(oldProg.man.ExportSectionWrap.GetSize())
	if oldProg.man.ExportSectionWrap.GetSize() == 0 {
		newModuleSize -= int64(oldRanges[section.Export].Size)
	}
	newModuleSize -= int64(oldRanges[section.Start].Size)
	newModuleSize -= int64(oldProg.man.BufferSection.GetSize())
	newModuleSize -= int64(oldProg.man.StackSection.GetSize())
	newModuleSize -= int64(oldRanges[section.Data].Size)

	newModuleSize += int64(len(memorySection))
	newModuleSize += int64(len(globalSection))
	newModuleSize += int64(len(snapshotSection))
	newModuleSize += int64(exportSectionWrap.GetSize())
	if exportSectionWrap.GetSize() == 0 {
		newModuleSize += int64(newRanges[section.Export].Size)
	}
	newModuleSize += int64(bufferSectionSize)
	newModuleSize += int64(stackSectionLen)
	newModuleSize += int64(dataSectionLen)

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
		if newRanges[i].Size != 0 {
			copyLen = int(newRanges[i].End())
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
	oldOff += int64(oldRanges[section.Memory].Size) + int64(oldRanges[section.Global].Size)

	// Write new snapshot section, and skip old one.
	n, err = newFile.WriteAt(snapshotSection, newOff)
	if err != nil {
		return nil, err
	}
	newOff += int64(n)
	oldOff += int64(oldProg.man.SnapshotSection.GetSize())

	// Copy export section, possibly writing or skipping wrapper.
	copyLen = int(oldRanges[section.Export].Size)
	if exportSectionWrap.GetSize() != 0 {
		if oldSize := oldProg.man.ExportSectionWrap.GetSize(); oldSize != 0 {
			copyLen = int(oldSize)
		} else {
			n, err = newFile.WriteAt(exportSectionWrapFrame, newOff)
			if err != nil {
				return nil, err
			}
			newOff += int64(n)
		}
	} else {
		if oldSize := oldProg.man.ExportSectionWrap.GetSize(); oldSize != 0 {
			oldOff += int64(oldSize) - int64(oldRanges[section.Export].Size)
		}
	}
	if err := copyFileRange(oldProg.file, &oldOff, newFile, &newOff, copyLen); err != nil {
		return nil, err
	}

	// Skip start section.
	oldOff += int64(oldRanges[section.Start].Size)

	// Copy element and code sections.
	copyLen = int(oldRanges[section.Element].Size) + int(oldRanges[section.Code].Size)
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
	oldOff += int64(oldProg.man.BufferSection.GetSize())

	// Write new stack section, and skip old one.
	if stackSectionLen > 0 {
		newStackSection := make([]byte, stackSectionLen)
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
	oldOff += int64(oldProg.man.StackSection.GetSize())

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
	oldOff += int64(oldRanges[section.Data].Size)

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
				Start: snapshotSectionOffset,
				Size:  uint32(len(snapshotSection)),
			},
			ExportSectionWrap: exportSectionWrap,
			BufferSection: &manifest.ByteRange{
				Start: bufferSectionOffset,
				Size:  bufferSectionSize,
			},
			BufferSectionHeaderSize: uint32(len(bufferHeader)),
			StackSection: &manifest.ByteRange{
				Start: stackSectionOffset,
				Size:  uint32(stackSectionLen),
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
		// Section id, payload size, item count.
		maxHeaderSize = 1 + binary.MaxVarintLen32 + binary.MaxVarintLen32

		// Type, mutable flag, const op, const value, end op.
		maxItemSize = 1 + 1 + 1 + binary.MaxVarintLen64 + 1
	)

	buf := make([]byte, maxHeaderSize+len(globalTypes)*maxItemSize)

	// Items:
	itemsLen := putGlobals(buf[maxHeaderSize:], globalTypes, segment)

	// Header:
	countLen := putVaruint32Before(buf, maxHeaderSize, uint32(len(globalTypes)))
	payloadLen := countLen + itemsLen
	payloadSizeLen := putVaruint32Before(buf, maxHeaderSize-countLen, uint32(payloadLen))
	buf[maxHeaderSize-countLen-payloadSizeLen-1] = byte(section.Global)

	return buf[maxHeaderSize-countLen-payloadSizeLen-1 : maxHeaderSize+itemsLen]
}

func putGlobals(target []byte, globalTypes []byte, segment []byte) (totalLen int) {
	for _, b := range globalTypes {
		t := wa.GlobalType(b)

		value := binary.LittleEndian.Uint64(segment)
		segment = segment[8:]

		encoded := t.Encode()
		n := copy(target, encoded[:])
		target = target[n:]
		totalLen += n

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
		totalLen += n

		target[0] = byte(opcode.End)
		target = target[1:]
		totalLen++
	}

	return
}

func makeSnapshotSection(snap *manifest.Snapshot) []byte {
	manifest.InflateSnapshot(&snap)

	// Section id, payload size.
	const maxSectionFrameSize = 1 + binary.MaxVarintLen32

	var maxPayloadLen = (0 +
		1 + // Name length
		len(wasm.SectionSnapshot) + // Name string
		1 + // Snapshot version
		binary.MaxVarintLen64 + // Flags
		binary.MaxVarintLen32 + // Trap
		binary.MaxVarintLen32 + // Result
		binary.MaxVarintLen64 + // Monotonic time
		binary.MaxVarintLen32 + // Breakpoint count
		binary.MaxVarintLen64*len(snap.Breakpoints)) // Breakpoint array

	b := make([]byte, maxSectionFrameSize+maxPayloadLen)
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

	payloadLen := i - maxSectionFrameSize
	// TODO: check if payloadLen is out of bounds

	payloadSizeLen := putVaruint32Before(b, maxSectionFrameSize, uint32(payloadLen))
	b[maxSectionFrameSize-payloadSizeLen-1] = byte(section.Custom)
	return b[maxSectionFrameSize-payloadSizeLen-1 : i]
}

func makeExportSectionWrapFrame(exportSectionSize uint32) []byte {
	// Section id, payload size.
	const maxSectionFrameSize = 1 + binary.MaxVarintLen32

	// Name length, name string.
	var nameHeaderLen = 1 + len(wasm.SectionExport)

	b := make([]byte, maxSectionFrameSize+nameHeaderLen)
	i := maxSectionFrameSize
	b[i] = byte(len(wasm.SectionExport))
	i++
	i += copy(b[i:], wasm.SectionExport)

	payloadSize := uint64(nameHeaderLen) + uint64(exportSectionSize)
	// TODO: check if payloadSize is out of bounds

	payloadSizeLen := putVaruint32Before(b, maxSectionFrameSize, uint32(payloadSize))
	b[maxSectionFrameSize-payloadSizeLen-1] = byte(section.Custom)
	return b[maxSectionFrameSize-payloadSizeLen-1 : i]
}

func makeBufferSectionHeader(buffers snapshot.Buffers) ([]byte, uint32) {
	if len(buffers.Services) == 0 && len(buffers.Input) == 0 && len(buffers.Output) == 0 {
		return nil, 0
	}

	// Section id, payload size.
	const maxSectionFrameSize = 1 + varint.MaxLen

	maxHeaderLen := maxSectionFrameSize
	maxHeaderLen += 1                       // Section name length
	maxHeaderLen += len(wasm.SectionBuffer) // Section name
	maxHeaderLen += varint.MaxLen           // Input data size
	maxHeaderLen += varint.MaxLen           // Output data size
	maxHeaderLen += varint.MaxLen           // Service count

	for _, s := range buffers.Services {
		maxHeaderLen += 1             // Service name length
		maxHeaderLen += len(s.Name)   // Service name
		maxHeaderLen += varint.MaxLen // Service data size
	}

	buf := make([]byte, maxHeaderLen)

	tail := buf[maxSectionFrameSize:]
	tail = putByte(tail, byte(len(wasm.SectionBuffer)))
	tail = putString(tail, wasm.SectionBuffer)
	tail = varint.Put(tail, int32(len(buffers.Input)))
	tail = varint.Put(tail, int32(len(buffers.Output)))
	tail = varint.Put(tail, int32(len(buffers.Services)))

	dataLen := len(buffers.Input) + len(buffers.Output)

	for _, s := range buffers.Services {
		tail = putByte(tail, byte(len(s.Name)))
		tail = putString(tail, s.Name)
		tail = varint.Put(tail, int32(len(s.Buffer)))

		dataLen += len(s.Buffer)
	}

	payloadLen := len(buf) - len(tail) - maxSectionFrameSize + dataLen
	// TODO: check if payloadLen is out of bounds

	payloadSizeLen := putVaruint32Before(buf, maxSectionFrameSize, uint32(payloadLen))
	buf[maxSectionFrameSize-payloadSizeLen-1] = byte(section.Custom)
	buf = buf[maxSectionFrameSize-payloadSizeLen-1:]

	header := buf[:len(buf)-len(tail)]
	sectionLen := len(header) + dataLen
	// TODO: check if sectionLen is out of bounds

	return header, uint32(sectionLen)
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
	// Section id, payload size.
	const maxSectionFrameSize = 1 + binary.MaxVarintLen32

	// Name length, name string.
	var customHeaderSize = 1 + len(wasm.SectionStack)

	buf := make([]byte, maxSectionFrameSize+customHeaderSize)
	buf[0] = byte(section.Custom)
	payloadSizeLen := binary.PutUvarint(buf[1:], uint64(customHeaderSize+stackSize))
	buf[1+payloadSizeLen] = byte(len(wasm.SectionStack))
	copy(buf[1+payloadSizeLen+1:], wasm.SectionStack)

	return buf[:1+payloadSizeLen+1+len(wasm.SectionStack)]
}

func makeDataSectionHeader(memorySize int) []byte {
	const (
		// Section id, payload size.
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
	segmentHeaderLen := 5 + binary.PutUvarint(segment[5:], uint64(memorySize))

	payloadLen := segmentHeaderLen + memorySize
	// TODO: check if payloadLen is out of bounds

	payloadSizeLen := putVaruint32Before(buf, maxSectionFrameSize, uint32(payloadLen))
	buf[maxSectionFrameSize-payloadSizeLen-1] = byte(section.Data)

	return buf[maxSectionFrameSize-payloadSizeLen-1 : maxSectionFrameSize+segmentHeaderLen]
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
	if src[i].Size != 0 {
		dest[i] = &manifest.ByteRange{
			Start: offset,
			Size:  src[i].Size,
		}
		offset += int64(src[i].Size)
	}
	return offset
}

func mapNewSection(offset int64, dest []*manifest.ByteRange, size uint32, i section.ID) int64 {
	if size != 0 {
		dest[i] = &manifest.ByteRange{
			Start: offset,
			Size:  size,
		}
	}
	return offset + int64(size)
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
