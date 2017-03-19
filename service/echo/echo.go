package echo

import "github.com/tsavola/gate/service"

const (
	Name    = "echo"
	Version = 0
)

func Register(r *service.Registry) {
	service.RegisterFunc(r, Name, Version, New)
}

func New(c chan<- []byte) service.Instance {
	return echo(c)
}

type echo chan<- []byte

func (c echo) Message(buf []byte) {
	c <- buf
}

func (c echo) Shutdown() {
}
