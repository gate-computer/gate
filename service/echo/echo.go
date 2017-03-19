package echo

import "github.com/tsavola/gate/service"

const (
	Name    = "echo"
	Version = 0
)

func Register(r *service.Registry) {
	service.RegisterFunc(r, Name, Version, New)
}

func New() service.Instance {
	return echo{}
}

type echo struct{}

func (echo) Handle(op []byte, evs chan<- []byte) {
	evs <- op
}

func (echo) Shutdown() {
}
