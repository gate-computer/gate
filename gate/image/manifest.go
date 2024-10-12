// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	pb "gate.computer/internal/pb/image"
	"gate.computer/wag/binding"
	"gate.computer/wag/compile"
	"gate.computer/wag/section"
)

const maxManifestSize = 4096 // Including header.

func byteRangeEnd(x *pb.ByteRange) int64 {
	return x.Start + int64(x.Size)
}

func getByteRangeEnd(x *pb.ByteRange) int64 {
	if x != nil {
		return byteRangeEnd(x)
	}
	return 0
}

func initProgramEntryFuncs(man *pb.ProgramManifest, mod compile.Module, funcAddrs []uint32) {
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

func programEntryFunc(man *pb.ProgramManifest, entryIndex int) *pb.Function {
	if entryIndex < 0 {
		return nil
	}

	addr, found := man.EntryAddrs[uint32(entryIndex)]
	if !found {
		panic(entryIndex)
	}

	return &pb.Function{
		Index: uint32(entryIndex),
		Addr:  addr,
	}
}

// programSectionsEnd returns the end offset of last standard or Gate section.
func programSectionsEnd(man *pb.ProgramManifest) int64 {
	end := int64(wasmModuleHeaderSize)

	for i := section.Type; i <= section.Data; i++ {
		if man.Sections[i].GetSize() > 0 {
			end = max(end, getByteRangeEnd(man.Sections[i]))
		}
	}

	if man.SnapshotSection.GetSize() > 0 {
		end = max(end, getByteRangeEnd(man.SnapshotSection))
	}
	if man.ExportSectionWrap.GetSize() > 0 {
		end = max(end, getByteRangeEnd(man.ExportSectionWrap))
	}
	if man.BufferSection.GetSize() > 0 {
		end = max(end, getByteRangeEnd(man.BufferSection))
	}
	if man.StackSection.GetSize() > 0 {
		end = max(end, getByteRangeEnd(man.StackSection))
	}

	return end
}
