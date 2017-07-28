package server

import (
	"io"

	"github.com/tsavola/wag/wasm"

	"github.com/tsavola/gate/run"
)

type Settings struct {
	MemorySizeLimit wasm.MemorySize
	StackSize       int32
	Env             *run.Environment
	Services        func(io.Reader, io.Writer) run.ServiceRegistry
	Log             Logger
	Debug           io.Writer
}
