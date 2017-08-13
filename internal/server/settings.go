package server

import (
	"github.com/tsavola/wag/wasm"
)

const (
	DefaultMemorySizeLimit = 256 * int(wasm.Page)
	DefaultStackSize       = 65536
	DefaultPreforkProcs    = 1

	maxStackSize = 0x40000000 // crazy but valid
)

type Settings struct {
	MemorySizeLimit int
	StackSize       int
	PreforkProcs    int
}

func (s *Settings) memorySizeLimit() wasm.MemorySize {
	if s.MemorySizeLimit > 0 {
		return (wasm.MemorySize(s.MemorySizeLimit) + (wasm.Page - 1)) &^ (wasm.Page - 1)
	} else {
		return wasm.MemorySize(DefaultMemorySizeLimit)
	}
}

func (s *Settings) stackSize() int32 {
	switch {
	case s.StackSize > maxStackSize:
		return maxStackSize

	case s.StackSize > 0:
		return int32(s.StackSize)

	default:
		return DefaultStackSize
	}
}

func (s *Settings) preforkProcs() int {
	if s.PreforkProcs > 0 {
		return s.PreforkProcs
	} else {
		return DefaultPreforkProcs
	}
}
