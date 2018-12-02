// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package abi

import (
	"github.com/tsavola/gate/internal/error/badprogram"
	"github.com/tsavola/wag/binding"
	"github.com/tsavola/wag/compile"
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
		"debug":    {-5, wa.FuncType{Params: []wa.Type{wa.I32, wa.I32}}},
		"exit":     {-4, wa.FuncType{Params: []wa.Type{wa.I32}}},
		"io.65536": {-3, wa.FuncType{Params: []wa.Type{wa.I32, wa.I32, wa.I32, wa.I32, wa.I32}}},
	},
	"env": {
		"__gate_debug":    {-5, wa.FuncType{Params: []wa.Type{wa.I32, wa.I32}}},
		"__gate_exit":     {-4, wa.FuncType{Params: []wa.Type{wa.I32}}},
		"__gate_io_65536": {-3, wa.FuncType{Params: []wa.Type{wa.I32, wa.I32, wa.I32, wa.I32, wa.I32}}},
	},
}

type resolver struct{}

func (resolver) ResolveFunc(module, field string, sig wa.FuncType) (index int, err error) {
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

func (resolver) ResolveGlobal(module, field string, t wa.Type) (value uint64, err error) {
	err = badprogram.Errorf("import global not supported: %q %q %s", module, field, t)
	return
}

func BindImports(module *compile.Module) error {
	return binding.BindImports(module, resolver{})
}
