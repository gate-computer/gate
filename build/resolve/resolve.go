// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package resolve

import (
	"github.com/tsavola/gate/internal/error/notfound"
	"github.com/tsavola/wag/binding"
	"github.com/tsavola/wag/compile"
)

func EntryFunc(mod compile.Module, exportName string) (index int, err error) {
	startIndex, startSig, startFound := mod.ExportFunc("_start")

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

	i, sig, ok := mod.ExportFunc(exportName)
	if !ok {
		return -1, notfound.ErrFunction
	}

	if !binding.IsEntryFuncType(sig) {
		return -1, notfound.ErrFunction
	}

	return int(i), nil
}
