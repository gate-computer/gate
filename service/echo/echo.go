package echo

import (
	"sync/atomic"

	"github.com/tsavola/gate/service"
)

const (
	Name    = "echo"
	Version = 0

	packetHeaderSize = 8
)

type Logger interface {
	Printf(string, ...interface{})
}

var prevId uint64 // atomic

type Factory struct {
	Log Logger
}

func (f *Factory) New() service.Instance {
	return &echo{
		id:  atomic.AddUint64(&prevId, 1),
		log: f.Log,
	}
}

var Default = new(Factory)

func Register(r *service.Registry) {
	service.Register(r, Name, Version, Default)
}

type echo struct {
	id  uint64
	log Logger
}

func (e *echo) Handle(buf []byte, replies chan<- []byte) {
	replies <- buf

	if e.log != nil {
		msg := buf[packetHeaderSize:]
		e.log.Printf("instance %d: %#v", e.id, string(msg))
	}
}

func (e *echo) Shutdown() {
	if e.log != nil {
		e.log.Printf("instance %d: shutdown", e.id)
	}
}
