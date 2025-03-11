// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"encoding/binary"
	"syscall"

	"gate.computer/gate/snapshot"
	"gate.computer/gate/snapshot/wasm"
	"gate.computer/internal/file"
	pb "gate.computer/internal/pb/image"
	"gate.computer/internal/varint"
	"gate.computer/wag/section"
	"gate.computer/wag/wa"
	"gate.computer/wag/wa/opcode"
)

const (
	wasmModuleHeaderSize = 8
	snapshotVersion      = 0
)

// Snapshot creates a new program from an instance.  The instance must not be
// running.
func Snapshot(oldProg *Program, inst *Instance, buffers *snapshot.Buffers, suspended bool) (newProg *Program, err error) {
	err = z.Recover(func() {
		s := mustNewState(oldProg, inst, buffers, suspended)
		defer s.mustClose()
		m := mustNewModuleState(s)
		newProg = mustSerializeState(s, m)
	})
	return
}

// state of an instance.
type state struct {
	prog    *Program
	inst    *Instance
	buffers *snapshot.Buffers

	initStack bool
	textAddr  uint64

	instMap []byte
	stack   []byte
	globals []byte // Aligned against end.
	memory  []byte
}

func mustNewState(prog *Program, inst *Instance, buffers *snapshot.Buffers, suspended bool) *state {
	s := &state{
		prog:    prog,
		inst:    inst,
		buffers: buffers,
	}

	// Initial values: assume that there is no stack content and mapping starts
	// between stack and globals, at page boundary.
	var (
		fileOffset       = int64(inst.man.StackSize)
		mapStackOffset   = -1
		mapGlobalsOffset = 0
	)

	if suspended || inst.Final() {
		if inst.man.StackUsage != 0 {
			var (
				stackUsage         = int(inst.man.StackUsage)
				stackUsagePageSize = alignPageSize32(inst.man.StackUsage)
			)

			// Mapping starts before stack contents, at page boundary.
			fileOffset -= int64(stackUsagePageSize)
			mapStackOffset = stackUsagePageSize - stackUsage
			mapGlobalsOffset = stackUsagePageSize

			// Stack contains absolute return addresses.
			s.textAddr = inst.man.TextAddr
		} else if suspended {
			// Resume at virtual call site at beginning of enter routine.
			s.initStack = true
		}
	}

	var (
		mapMemoryOffset = mapGlobalsOffset + alignPageSize32(inst.man.GlobalsSize)
		mapSize         = mapMemoryOffset + int(inst.man.MemorySize)
	)

	s.instMap = must(mmap(inst.file.FD(), fileOffset, mapSize, syscall.PROT_READ, syscall.MAP_PRIVATE))

	if mapStackOffset >= 0 {
		s.stack = s.instMap[mapStackOffset:mapGlobalsOffset]
	}
	s.globals = s.instMap[mapGlobalsOffset:mapMemoryOffset]
	s.memory = s.instMap[mapMemoryOffset:]

	return s
}

func (s *state) mustClose() {
	mustMunmap(s.instMap)
}

func (s *state) hasStack() bool {
	return s.initStack || len(s.stack) > 0
}

// moduleState is a companion for state.
type moduleState struct {
	memory     []byte
	global     []byte
	snapshot   []byte
	exportWrap []byte
	bufferHead []byte
	bufferSize uint32
	stackHead  []byte
	stackData  []byte
	data       []byte

	ranges          []*pb.ByteRange
	rangeSnapshot   *pb.ByteRange
	rangeExportWrap *pb.ByteRange
	rangeBuffer     *pb.ByteRange
	rangeStack      *pb.ByteRange

	size int64
}

func mustNewModuleState(s *state) *moduleState {
	m := &moduleState{
		ranges: make([]*pb.ByteRange, section.Data+1),
	}

	// Section contents

	m.memory = makeMemorySection(s.inst.man.MemorySize, s.prog.man.MemorySizeLimit) // TODO: check size
	m.global = makeGlobalSection(s.prog.man.GlobalTypes, s.globals)                 // TODO: check size
	m.snapshot = makeSnapshotSection(s.inst.man.Snapshot)

	exportSectionSize := s.prog.man.Sections[section.Export].GetSize()
	if exportSectionSize > 0 && s.hasStack() {
		m.exportWrap = makeExportSectionWrapFrame(exportSectionSize)
	}

	m.bufferHead, m.bufferSize = makeBufferSectionHeader(s.buffers)

	if s.hasStack() {
		if s.initStack {
			m.stackData = makeInitStack(s.inst.man.StartFunc, s.inst.man.EntryFunc)
		} else {
			m.stackData = make([]byte, len(s.stack))
			z.Check(exportStack(m.stackData, s.stack, s.inst.man.TextAddr, &s.prog.Map))
		}
		m.stackHead = makeStackSectionHeader(len(m.stackData))
	}

	m.data = mustMakeDataSection(s.inst.file, s.inst.memoryOffset(), s.memory)

	// Section sizes

	for i := section.Type; i <= section.Data; i++ {
		if i != section.Start {
			m.ranges[i] = &pb.ByteRange{Size: s.prog.man.Sections[i].GetSize()}
		}
	}

	m.ranges[section.Memory].Size = uint32(len(m.memory))
	m.ranges[section.Global].Size = uint32(len(m.global))
	m.rangeSnapshot = &pb.ByteRange{Size: uint32(len(m.snapshot))}

	if wrapLen := len(m.exportWrap); wrapLen > 0 {
		m.rangeExportWrap = &pb.ByteRange{Size: uint32(wrapLen) + exportSectionSize}
	}

	if len(m.bufferHead) > 0 {
		m.rangeBuffer = &pb.ByteRange{Size: m.bufferSize}
	}

	if headLen := len(m.stackHead); headLen > 0 {
		m.rangeStack = &pb.ByteRange{Size: uint32(headLen + len(m.stackData))}
	}

	m.ranges[section.Data].Size = uint32(len(m.data)) // TODO: check size

	// Section offsets

	offset := int64(wasmModuleHeaderSize)

	for i := section.Type; i <= section.Table; i++ {
		if size := m.ranges[i].GetSize(); size > 0 {
			m.ranges[i].Start = offset
			offset += int64(size)
		}
	}

	m.ranges[section.Memory].Start = offset
	offset += int64(m.ranges[section.Memory].Size)

	if size := m.ranges[section.Global].GetSize(); size > 0 {
		m.ranges[section.Global].Start = offset
		offset += int64(size)
	}

	m.rangeSnapshot.Start = offset
	offset += int64(m.rangeSnapshot.Size)

	if size := m.rangeExportWrap.GetSize(); size > 0 {
		m.rangeExportWrap.Start = offset
		offset += int64(len(m.exportWrap)) // Don't skip wrappee.
	}
	if size := m.ranges[section.Export].GetSize(); size > 0 {
		m.ranges[section.Export].Start = offset
		offset += int64(size)
	}

	if size := m.ranges[section.Element].GetSize(); size > 0 {
		m.ranges[section.Element].Start = offset
		offset += int64(size)
	}

	if size := m.ranges[section.Code].GetSize(); size > 0 {
		m.ranges[section.Code].Start = offset
		offset += int64(size)
	}

	if len(m.bufferHead) > 0 {
		m.rangeBuffer.Start = offset
		offset += int64(m.rangeBuffer.Size)
	}

	if size := m.rangeStack.GetSize(); size > 0 {
		m.rangeStack.Start = offset
		offset += int64(size)
	}

	m.ranges[section.Data].Start = offset
	offset += int64(m.ranges[section.Data].Size)

	// Module size

	m.size = offset + (s.prog.man.ModuleSize - programSectionsEnd(s.prog.man))
	return m
}

func mustSerializeState(s *state, m *moduleState) *Program {
	prog := &Program{
		Map:     s.prog.Map,
		storage: s.prog.storage,
		man: &pb.ProgramManifest{
			LibraryChecksum:         s.prog.man.LibraryChecksum,
			TextRevision:            s.prog.man.TextRevision,
			TextAddr:                s.textAddr,
			TextSize:                s.prog.man.TextSize,
			StackUsage:              uint32(len(m.stackData)),
			GlobalsSize:             s.prog.man.GlobalsSize,
			MemorySize:              s.inst.man.MemorySize,
			MemorySizeLimit:         s.prog.man.MemorySizeLimit,
			MemoryDataSize:          s.inst.man.MemorySize,
			ModuleSize:              m.size,
			Sections:                m.ranges,
			SnapshotSection:         m.rangeSnapshot,
			ExportSectionWrap:       m.rangeExportWrap,
			BufferSection:           m.rangeBuffer,
			BufferSectionHeaderSize: uint32(len(m.bufferHead)),
			StackSection:            m.rangeStack,
			GlobalTypes:             s.prog.man.GlobalTypes,
			StartFunc:               s.prog.man.StartFunc,
			EntryIndexes:            s.prog.man.EntryIndexes,
			EntryAddrs:              s.prog.man.EntryAddrs,
			CallSitesSize:           s.prog.man.CallSitesSize,
			FuncAddrsSize:           s.prog.man.FuncAddrsSize,
			Random:                  s.prog.man.Random,
			Snapshot:                snapshot.Clone(s.prog.man.Snapshot),
		},
	}

	f := must(prog.storage.newProgramFile())
	defer func() {
		if f != nil {
			f.Close()
		}
	}()

	mustWriteState(f, s)
	mustWriteModuleState(f, s, m)

	prog.file = f
	f = nil
	return prog
}

func mustWriteState(f *file.File, s *state) {
	// Program text

	copySize := alignPageSize32(s.prog.man.TextSize)
	copyDest := progTextOffset
	copyFrom := progTextOffset

	z.Check(copyFileRange(s.prog.file, &copyFrom, f, &copyDest, copySize))

	// Instance stack, globals and memory

	copySize = alignPageSize32(s.inst.man.GlobalsSize) + alignPageSize32(s.inst.man.MemorySize)
	copyDest = progGlobalsOffset
	copyFrom = s.inst.globalsPageOffset()

	if len(s.stack) > 0 && s.textAddr != 0 {
		stackPageSize := alignPageSize(len(s.stack))
		copySize += stackPageSize
		copyDest -= int64(stackPageSize)
		copyFrom -= int64(stackPageSize)
	}

	z.Check(copyFileRange(s.inst.file, &copyFrom, f, &copyDest, copySize))
}

func mustWriteModuleState(f *file.File, s *state, m *moduleState) {
	dest := progModuleOffset

	copyFromModule := func(r *pb.ByteRange) {
		if r.GetSize() > 0 {
			from := progModuleOffset + r.Start
			z.Check(copyFileRange(s.prog.file, &from, f, &dest, int(r.Size)))
		}
	}

	copyFromModule(&pb.ByteRange{Start: 0, Size: wasmModuleHeaderSize})

	for i := section.Type; i <= section.Table; i++ {
		copyFromModule(s.prog.man.Sections[i])
	}

	dest += int64(must(f.WriteAt(m.memory, dest)))
	dest += int64(must(f.WriteAt(m.global, dest)))
	dest += int64(must(f.WriteAt(m.snapshot, dest)))

	if len(m.exportWrap) > 0 {
		dest += int64(must(f.WriteAt(m.exportWrap, dest)))
	}

	for i := section.Export; i <= section.Code; i++ {
		copyFromModule(s.prog.man.Sections[i])
	}

	if len(m.bufferHead) > 0 {
		dest += int64(must(f.WriteAt(m.bufferHead, dest)))
		dest += int64(must(writeBufferSectionDataAt(f, s.buffers, dest)))
	}

	if len(m.stackHead) > 0 {
		dest += int64(must(f.WriteAt(m.stackHead, dest)))
		dest += int64(must(f.WriteAt(m.stackData, dest)))
	}

	dest += int64(must(f.WriteAt(m.data, dest)))

	// Trailing custom sections
	from := programSectionsEnd(s.prog.man)
	copyFromModule(&pb.ByteRange{Start: from, Size: uint32(s.prog.man.ModuleSize - from)})
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

func makeGlobalSection(globalTypes, data []byte) []byte {
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
	itemsLen := putGlobals(buf[maxHeaderSize:], globalTypes, data)

	// Header:
	countLen := putVaruint32Before(buf, maxHeaderSize, uint32(len(globalTypes)))
	payloadLen := countLen + itemsLen
	payloadSizeLen := putVaruint32Before(buf, maxHeaderSize-countLen, uint32(payloadLen))
	buf[maxHeaderSize-countLen-payloadSizeLen-1] = byte(section.Global)

	return buf[maxHeaderSize-countLen-payloadSizeLen-1 : maxHeaderSize+itemsLen]
}

func putGlobals(target, globalTypes, data []byte) (totalLen int) {
	offset := len(data) - len(globalTypes)*8
	data = data[offset:]

	for _, b := range globalTypes {
		t := wa.GlobalType(b)

		value := binary.LittleEndian.Uint64(data)
		data = data[8:]

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

func makeSnapshotSection(snap *snapshot.Snapshot) []byte {
	// Section id, payload size.
	const maxSectionFrameSize = 1 + binary.MaxVarintLen32

	maxPayloadLen := (0 +
		1 + // Name length
		len(wasm.SectionSnapshot) + // Name string
		1 + // Snapshot version
		1 + // Final flag
		binary.MaxVarintLen32 + // Trap
		binary.MaxVarintLen32 + // Result
		binary.MaxVarintLen64 + // Monotonic time
		binary.MaxVarintLen32 + // Breakpoint count
		binary.MaxVarintLen64*len(snap.GetBreakpoints())) // Breakpoint array

	b := make([]byte, maxSectionFrameSize+maxPayloadLen)
	i := maxSectionFrameSize
	b[i] = byte(len(wasm.SectionSnapshot))
	i++
	i += copy(b[i:], wasm.SectionSnapshot)
	b[i] = snapshotVersion
	i++
	if snap.GetFinal() {
		b[i] = 1
	}
	i++
	i += binary.PutUvarint(b[i:], uint64(snap.GetTrap()))
	i += binary.PutUvarint(b[i:], uint64(snap.GetResult()))
	i += binary.PutUvarint(b[i:], snap.GetMonotonicTime())
	i += binary.PutUvarint(b[i:], uint64(len(snap.GetBreakpoints())))
	for _, offset := range snap.GetBreakpoints() {
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
	nameHeaderLen := 1 + len(wasm.SectionExport)

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

func makeBufferSectionHeader(buffers *snapshot.Buffers) ([]byte, uint32) {
	if len(buffers.GetServices()) == 0 && len(buffers.GetInput()) == 0 && len(buffers.GetOutput()) == 0 {
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

func writeBufferSectionDataAt(f *file.File, bs *snapshot.Buffers, off int64) (int, error) {
	n, err := f.WriteAt(bs.Input, off)
	total := n
	if err != nil {
		return total, err
	}
	off += int64(n)

	n, err = f.WriteAt(bs.Output, off)
	total += n
	if err != nil {
		return total, err
	}
	off += int64(n)

	for _, s := range bs.Services {
		n, err = f.WriteAt(s.Buffer, off)
		total += n
		if err != nil {
			return total, err
		}
		off += int64(n)
	}

	return total, nil
}

func makeStackSectionHeader(stackSize int) []byte {
	// Section id, payload size.
	const maxSectionFrameSize = 1 + binary.MaxVarintLen32

	// Name length, name string.
	customHeaderSize := 1 + len(wasm.SectionStack)

	buf := make([]byte, maxSectionFrameSize+customHeaderSize)
	buf[0] = byte(section.Custom)
	payloadSizeLen := binary.PutUvarint(buf[1:], uint64(customHeaderSize+stackSize))
	buf[1+payloadSizeLen] = byte(len(wasm.SectionStack))
	copy(buf[1+payloadSizeLen+1:], wasm.SectionStack)

	return buf[:1+payloadSizeLen+1+len(wasm.SectionStack)]
}

func putByte(dest []byte, x byte) (tail []byte) {
	dest[0] = x
	return dest[1:]
}

func putString(dest []byte, s string) (tail []byte) {
	copy(dest, s)
	return dest[len(s):]
}

func putVaruint32Before(dest []byte, offset int, x uint32) int {
	var temp [binary.MaxVarintLen32]byte
	n := binary.PutUvarint(temp[:], uint64(x))
	copy(dest[offset-n:], temp[:n])
	return n
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
