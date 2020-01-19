// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package manifest

import (
	"github.com/tsavola/wag/binding"
	"github.com/tsavola/wag/compile"
)

const MaxSize = 4096 // Including header.
const MaxBreakpoints = 100

var NoFunction = Function{Index: -1}

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

func (man Program) EntryFunc(entryIndex int) Function {
	if entryIndex < 0 {
		return NoFunction
	}

	addr, found := man.EntryAddrs[uint32(entryIndex)]
	if !found {
		panic(entryIndex)
	}

	return Function{
		Index: int64(entryIndex),
		Addr:  addr,
	}
}
