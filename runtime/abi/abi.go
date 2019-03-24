// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package abi

import (
	"github.com/tsavola/gate/internal/error/badprogram"
	"github.com/tsavola/wag/wa"
)

const (
	MaxVersion = 0
	MinVersion = 0
)

type function struct {
	index int
	sig   wa.FuncType
}

// Mirrors the vector initialization in runtime/loader/loader.c
var moduleFunctions = map[string]map[string]function{
	"gate": {
		"debug":    {-6, wa.FuncType{Params: []wa.Type{wa.I32, wa.I32}}},
		"exit":     {-5, wa.FuncType{Params: []wa.Type{wa.I32}}},
		"io.65536": {-4, wa.FuncType{Params: []wa.Type{wa.I32, wa.I32, wa.I32, wa.I32, wa.I32}}},
	},
	"env": {
		"__gate_debug":    {-6, wa.FuncType{Params: []wa.Type{wa.I32, wa.I32}}},
		"__gate_exit":     {-5, wa.FuncType{Params: []wa.Type{wa.I32}}},
		"__gate_io_65536": {-4, wa.FuncType{Params: []wa.Type{wa.I32, wa.I32, wa.I32, wa.I32, wa.I32}}},
	},
}

type ImportResolver struct {
	random bool
}

func (*ImportResolver) ResolveFunc(module, field string, sig wa.FuncType) (index int, err error) {
	if functions, found := moduleFunctions[module]; found {
		if f, found := functions[field]; found {
			if !f.sig.Equal(sig) {
				err = badprogram.Errorf("function %s.%s %s imported with wrong signature %s", module, field, f.sig, sig)
				return
			}

			index = f.index
			return
		}
	}

	err = badprogram.Errorf("import function not supported: %q %q %s", module, field, sig)
	return
}

func (ir *ImportResolver) ResolveGlobal(module, field string, t wa.Type) (value uint64, err error) {
	if module == "gate" && field == "random.8" && t == wa.I64 {
		ir.random = true
		// Value will be initialized separately for each instance.
		return
	}

	err = badprogram.Errorf("import global not supported: %q %q %s", module, field, t)
	return
}

func (ir *ImportResolver) RandomGlobal() (index int, imported bool) {
	if ir.random {
		// Imports precede other globals and this is the only one we support,
		// so it's always first.
		index = -1
		imported = true
	}
	return
}
