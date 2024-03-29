// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package build

import (
	"gate.computer/internal/error/notfound"
	"gate.computer/wag/binding"
	"gate.computer/wag/compile"
	"gate.computer/wag/wa"
)

// ResolveEntryFunc index or the implicit _start function index.  This function
// doesn't know if the module is a snapshot: the started argument must be true
// for snapshots.
func ResolveEntryFunc(mod compile.Module, exportName string, started bool) (int, error) {
	// image.Program.ResolveEntryFunc must be kept in sync with this.

	var (
		startIndex uint32
		startSig   wa.FuncType
		startFound bool
	)
	if !started {
		startIndex, startSig, startFound = mod.ExportFunc("_start")
	}

	if exportName == "" {
		if startFound && binding.IsEntryFuncType(startSig) {
			return int(startIndex), nil
		} else {
			return -1, nil
		}
	}

	if startFound {
		return -1, notfound.ErrStart
	}

	if exportName == "_start" {
		return -1, nil
	}

	i, sig, found := mod.ExportFunc(exportName)
	if !found || !binding.IsEntryFuncType(sig) {
		return -1, notfound.ErrFunction
	}

	return int(i), nil
}
