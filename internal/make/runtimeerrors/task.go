// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtimeerrors

import (
	"bytes"
	"fmt"
	"os"

	runtimeerrors "gate.computer/internal/error/runtime"
	m "import.name/make"
)

const source = "internal/make/runtimeerrors/task.go"

func Task(GOFMT string) m.Task {
	cTarget := "gate/runtime/include/errors.gen.hpp"
	gTarget := "internal/error/runtime/const.gen.go"
	deps := m.Globber("internal/error/runtime/*.go", source)

	return m.If(
		m.Any(
			m.Outdated(cTarget, deps),
			m.Outdated(gTarget, deps),
		),
		m.Func(func() error {
			m.Println("Making", gTarget, "and", cTarget)

			errorGroups := [][]runtimeerrors.Error{
				runtimeerrors.ExecutorErrors[:],
				runtimeerrors.ProcessErrors[:],
			}

			c := bytes.NewBuffer(nil)
			g := bytes.NewBuffer(nil)

			fmt.Fprintf(c, "// Code generated by %s, DO NOT EDIT!\n", source)
			fmt.Fprintln(c)
			fmt.Fprintln(c, "#pragma once")

			fmt.Fprintf(g, "// Code generated by %s, DO NOT EDIT!\n", source)
			fmt.Fprintln(g)
			fmt.Fprintln(g, "package runtime")

			for _, errors := range errorGroups {
				fmt.Fprintln(c)

				fmt.Fprintln(g)
				fmt.Fprintln(g, "const (")

				for i, e := range errors {
					if e.Define != "" {
						fmt.Fprintf(c, "#define %s %d\n", e.Define, i)
						fmt.Fprintf(g, "%s = %d\n", e.Define, i)
					}
				}

				fmt.Fprintln(g, ")")
			}

			gFmt, err := m.RunIO(g, GOFMT)
			if err != nil {
				return err
			}

			if err := os.WriteFile(cTarget, c.Bytes(), 0o666); err != nil {
				return err
			}

			if err := os.WriteFile(gTarget, gFmt, 0o666); err != nil {
				return err
			}

			return nil
		}),
	)
}
