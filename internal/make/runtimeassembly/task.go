// Copyright (c) 2022 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtimeassembly

import (
	"path"

	m "import.name/make"
)

func Task(GO string) m.Task {
	deps := m.Globber(
		"internal/error/runtime/*.go",
		"internal/make/runtimeassembly/*.go",
	)

	var (
		archs     = []string{"amd64", "arm64"}
		filenames = []string{"runtime.S", "runtime-android.S"}
	)

	var conds []func() bool
	for _, arch := range archs {
		for _, filename := range filenames {
			filename = path.Join("runtime/loader", arch, filename)
			conds = append(conds, m.Outdated(filename, deps))
		}
	}

	return m.If(
		m.Any(conds...),
		m.Command(GO, "run", "internal/make/runtimeassembly/main.go"),
	)
}
