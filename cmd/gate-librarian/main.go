// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"os"

	"gate.computer/internal/librarian"
)

const usage = `Usage: %s [options] filename [-- command... [-- command...] ...]

WebAssembly module is read from stdin, or from the stdout of command(s).
WebAssembly or Go code (depending on options) is written to filename.
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

	ld := os.Getenv("WASM_LD")
	if ld == "" {
		ld = "wasm-ld"
	}
	objdump := os.Getenv("WASM_OBJDUMP")
	if objdump == "" {
		objdump = "wasm-objdump"
	}

	var (
		gopkg   string
		verbose bool
	)

	flag.BoolVar(&verbose, "v", verbose, "don't be quiet")
	flag.StringVar(&gopkg, "go", gopkg, "generate Go code for given package")
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

	if err := librarian.Build(output, ld, objdump, gopkg, verbose, commands); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
