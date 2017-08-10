package server

import (
	"io"

	"github.com/tsavola/wag/wasm"

	"github.com/tsavola/gate/run"
)

type Settings struct {
	Env               *run.Environment
	Services          func(io.Reader, io.Writer) run.ServiceRegistry
	MemorySizeLimit   wasm.MemorySize
	StackSize         int32
	ProcessPreforkNum int
	Log               Logger
	Debug             io.Writer
}
