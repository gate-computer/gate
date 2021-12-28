// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package rt defines a public subset of the runtime library ABI.
package rt

// ImportFuncs returns a map of modules, the values of which are maps of
// function vector indexes.
func ImportFuncs() map[string]map[string]int {
	return functions
}

// Mirrors the vector initialization in runtime/loader/loader.cpp
var functions = map[string]map[string]int{
	"env": {
		"rt_write8": -19,
		"rt_read8":  -18,
		"rt_trap":   -17,
		"rt_debug":  -16,
	},
}
