package serverconfig

import (
	"io"

	"github.com/tsavola/gate/run"
)

type Options struct {
	Env      *run.Environment
	Services func(io.Reader, io.Writer) run.ServiceRegistry
	Log      Logger
	Debug    io.Writer
}
