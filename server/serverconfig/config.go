package serverconfig

import (
	"io"

	"github.com/tsavola/gate/run"
)

const (
	DefaultMemorySizeLimit = 16777216
	DefaultStackSize       = 65536
	DefaultPreforkProcs    = 1
)

type Config struct {
	Env      *run.Environment
	Services func(io.Reader, io.Writer) run.ServiceRegistry
	Log      Logger
	Debug    io.Writer

	MemorySizeLimit int
	StackSize       int
	PreforkProcs    int
}
