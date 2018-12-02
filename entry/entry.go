// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package entry contains entry function utilities.
package entry

import (
	"github.com/tsavola/gate/internal/error/notfound"
	"github.com/tsavola/wag/compile"
	"github.com/tsavola/wag/wa"
)

func FuncIndex(mod *compile.Module, name string) (index uint32, err error) {
	index, sig, ok := mod.ExportFunc(name)
	if ok {
		ok = checkType(sig)
	}
	if !ok {
		err = notfound.ErrFunction
	}
	return
}

func FuncAddrs(mod *compile.Module, funcAddrs []uint32) (entryAddrs map[string]uint32) {
	entryAddrs = make(map[string]uint32)

	sigs := mod.Types()
	sigIndexes := mod.FuncTypeIndexes()

	for name, funcIndex := range mod.ExportFuncs() {
		sigIndex := sigIndexes[funcIndex]
		sig := sigs[sigIndex]

		if checkType(sig) {
			entryAddrs[name] = funcAddrs[funcIndex]
		}
	}

	return
}

func FuncAddr(entryAddrs map[string]uint32, name string) (addr uint32, err error) {
	addr, ok := entryAddrs[name]
	if !ok {
		err = notfound.ErrFunction
	}
	return
}

func checkType(sig wa.FuncType) bool {
	return len(sig.Params) == 0 && (sig.Result == wa.Void || sig.Result == wa.I32)
}
