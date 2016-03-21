package main

import (
	"debug/elf"
	"os"

	"../run"
)

const (
	memorySize = 256 * 1024 * 1024 // XXX
)

func main() {
	elfFile, err := elf.NewFile(os.Stdin)
	if err != nil {
		panic(err)
	}

	payload, err := run.NewPayload(elfFile, memorySize)
	if err != nil {
		panic(err)
	}

	err = payload.WriteTo(os.Stdout)
	if err != nil {
		panic(err)
	}
}
