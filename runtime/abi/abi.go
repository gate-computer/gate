// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package abi

import (
	"github.com/tsavola/gate/internal/error/badprogram"
	"github.com/tsavola/wag/wa"
)

type function struct {
	index  int
	sig    wa.FuncType
	random bool
}

// Mirrors the vector initialization in runtime/loader/loader.c
var moduleFunctions = map[string]map[string]function{
	"gate": {
		"debug":      {-8, wa.FuncType{Params: []wa.Type{wa.I32, wa.I32}}, false},
		"exit":       {-11, wa.FuncType{Params: []wa.Type{wa.I32}}, false},
		"io.65536":   {-4, wa.FuncType{Params: []wa.Type{wa.I32, wa.I32, wa.I32, wa.I32, wa.I32}}, false},
		"randomseed": {-9, wa.FuncType{Result: wa.I64}, true},
		"time":       {-5, wa.FuncType{Result: wa.I32, Params: []wa.Type{wa.I32, wa.I32}}, false},
	},
	"env": {
		"__gate_debug":      {-8, wa.FuncType{Params: []wa.Type{wa.I32, wa.I32}}, false},
		"__gate_exit":       {-11, wa.FuncType{Params: []wa.Type{wa.I32}}, false},
		"__gate_io_65536":   {-4, wa.FuncType{Params: []wa.Type{wa.I32, wa.I32, wa.I32, wa.I32, wa.I32}}, false},
		"__gate_randomseed": {-9, wa.FuncType{Result: wa.I64}, true},
		"__gate_time":       {-5, wa.FuncType{Result: wa.I32, Params: []wa.Type{wa.I32, wa.I32}}, false},
	},
}

type ImportResolver struct {
	RandomSeed bool
}

func (ir *ImportResolver) ResolveFunc(module, field string, sig wa.FuncType) (index int, err error) {
	if functions, found := moduleFunctions[module]; found {
		if f, found := functions[field]; found {
			if !f.sig.Equal(sig) {
				err = badprogram.Errorf("function %s.%s %s imported with wrong signature %s", module, field, f.sig, sig)
				return
			}

			if f.random {
				ir.RandomSeed = true
			}

			index = f.index
			return
		}
	}

	err = badprogram.Errorf("import function not supported: %q %q %s", module, field, sig)
	return
}

func (*ImportResolver) ResolveGlobal(module, field string, t wa.Type) (value uint64, err error) {
	err = badprogram.Errorf("import global not supported: %q %q %s", module, field, t)
	return
}
