// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package entry contains start and entry function utilities.
package entry

import (
	internal "github.com/tsavola/gate/internal/entry"
	"github.com/tsavola/gate/internal/error/notfound"
	"github.com/tsavola/wag/compile"
	"github.com/tsavola/wag/wa"
)

const StartFuncName = "_start"

var StartFuncType wa.FuncType

// ModuleFuncIndex returns an error if name is not exported by module or has
// incompatible type.
func ModuleFuncIndex(mod compile.Module, name string) (index uint32, err error) {
	if name == StartFuncName {
		err = notfound.ErrFunction
		return
	}

	index, sig, ok := mod.ExportFunc(name)
	if ok {
		ok = internal.CheckType(sig)
	}
	if !ok {
		err = notfound.ErrFunction
	}
	return
}

// Maps always succeeds.  entryAddrs will contain all entryIndexes.
func Maps(mod compile.Module, funcAddrs []uint32,
) (entryIndexes map[string]uint32, entryAddrs map[uint32]uint32) {
	entryIndexes = make(map[string]uint32)
	entryAddrs = make(map[uint32]uint32)

	sigs := mod.Types()
	sigIndexes := mod.FuncTypeIndexes()

	for name, funcIndex := range mod.ExportFuncs() {
		sigIndex := sigIndexes[funcIndex]
		sig := sigs[sigIndex]

		if internal.CheckType(sig) {
			entryIndexes[name] = funcIndex
			entryAddrs[funcIndex] = funcAddrs[funcIndex]
		}
	}

	return
}

// MapFuncIndex returns an error if name is not in entryIndexes.
func MapFuncIndex(entryIndexes map[string]uint32, name string) (index uint32, err error) {
	if name == StartFuncName {
		err = notfound.ErrFunction
		return
	}

	index, ok := entryIndexes[name]
	if !ok {
		err = notfound.ErrFunction
	}
	return
}

// MapFuncAddr panics if index is not in entryAddrs.
func MapFuncAddr(entryAddrs map[uint32]uint32, index uint32) (addr uint32) {
	addr, ok := entryAddrs[index]
	if !ok {
		panic(index)
	}
	return
}
