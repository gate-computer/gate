// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package manifest

import (
	"gate.computer/wag/binding"
	"gate.computer/wag/compile"
	"gate.computer/wag/section"
)

const wasmModuleHeaderLen = 8

const (
	MaxSize        = 4096 // Including header.
	MaxBreakpoints = 100
)

func (x *ByteRange) End() int64 {
	return x.Start + int64(x.Size)
}

func (x *ByteRange) GetEnd() int64 {
	if x != nil {
		return x.End()
	}
	return 0
}

func (man *Program) InitEntryFuncs(mod compile.Module, funcAddrs []uint32) {
	man.EntryIndexes = make(map[string]uint32)
	man.EntryAddrs = make(map[uint32]uint32)

	sigs := mod.Types()
	sigIndexes := mod.FuncTypeIndexes()

	for name, funcIndex := range mod.ExportFuncs() {
		sigIndex := sigIndexes[funcIndex]
		sig := sigs[sigIndex]

		if binding.IsEntryFuncType(sig) {
			man.EntryIndexes[name] = funcIndex
			man.EntryAddrs[funcIndex] = funcAddrs[funcIndex]
		}
	}
}

func (man *Program) EntryFunc(entryIndex int) *Function {
	if entryIndex < 0 {
		return nil
	}

	addr, found := man.EntryAddrs[uint32(entryIndex)]
	if !found {
		panic(entryIndex)
	}

	return &Function{
		Index: uint32(entryIndex),
		Addr:  addr,
	}
}

// SectionsEnd returns the end offset of last standard or Gate section.
func (man *Program) SectionsEnd() int64 {
	end := int64(wasmModuleHeaderLen)

	for i := section.Type; i <= section.Data; i++ {
		if man.Sections[i].GetSize() > 0 {
			end = max(end, man.Sections[i].GetEnd())
		}
	}

	if man.SnapshotSection.GetSize() > 0 {
		end = max(end, man.SnapshotSection.GetEnd())
	}
	if man.ExportSectionWrap.GetSize() > 0 {
		end = max(end, man.ExportSectionWrap.GetEnd())
	}
	if man.BufferSection.GetSize() > 0 {
		end = max(end, man.BufferSection.GetEnd())
	}
	if man.StackSection.GetSize() > 0 {
		end = max(end, man.StackSection.GetEnd())
	}

	return end
}

func InflateSnapshot(s **Snapshot) *Snapshot {
	if *s == nil {
		*s = new(Snapshot)
	}
	return *s
}

func (s *Snapshot) Clone() *Snapshot {
	if s == nil {
		return nil
	}
	return &Snapshot{
		Flags:         s.Flags,
		Trap:          s.Trap,
		Result:        s.Result,
		MonotonicTime: s.MonotonicTime,
		Breakpoints:   append([]uint64(nil), s.Breakpoints...),
	}
}
