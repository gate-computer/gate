// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"os"
	"path"

	"gate.computer/internal/librarian"
)

const usage = `Usage: %s [options] wasm-file [-- command... [-- command...] ...]

WebAssembly module is read from stdin, or from the stdout of command(s).
WebAssembly and Go code (depending on options) is written to filename.
Multiple objects can be linked by specifying multiple commands.

Options:
`

func badUsage() {
	flag.Usage()
	os.Exit(2)
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), usage, flag.CommandLine.Name())
		flag.PrintDefaults()
	}

	var (
		gosrc string
		gopkg = "main"
	)

	ld := os.Getenv("WASM_LD")
	if ld == "" {
		ld = "wasm-ld"
	}

	objdump := os.Getenv("WASM_OBJDUMP")
	if objdump == "" {
		objdump = "wasm-objdump"
	}

	flag.StringVar(&gosrc, "go", gosrc, "generate Go source file which embeds the WASM file")
	flag.StringVar(&gopkg, "pkg", gopkg, "set package name for generated Go source")
	flag.StringVar(&ld, "ld", ld, "wasm-ld command to use")
	flag.StringVar(&objdump, "objdump", objdump, "wasm-objdump command to use")
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		badUsage()
	}
	output := args[0]
	args = args[1:]

	var commands [][]string

	for len(args) > 0 {
		if args[0] != "--" {
			badUsage()
		}
		args = args[1:]

		var cmd []string

		for len(args) > 0 && args[0] != "--" {
			cmd = append(cmd, args[0])
			args = args[1:]
		}

		if len(cmd) == 0 {
			badUsage()
		}

		commands = append(commands, cmd)
	}

	if gosrc != "" && path.Dir(gosrc) != path.Dir(output) {
		fmt.Fprintln(os.Stderr, "Go and WASM files must be in same directory")
		os.Exit(2)
	}

	if err := librarian.Build(output, ld, objdump, gosrc, gopkg, commands); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
