package origin

import (
	"io"

	"github.com/tsavola/gate/service"
)

const (
	Name    = "origin"
	Version = 0

	packetHeaderSize = 8
)

type Factory struct {
	R io.Reader
	W io.Writer
}

func (f *Factory) Register(r *service.Registry) {
	service.Register(r, Name, Version, f)
}

func (f *Factory) New() service.Instance {
	return &origin{f.R, f.W}
}

var Default = new(Factory)

func Register(r *service.Registry) {
	Default.Register(r)
}

func CloneRegistryWith(r *service.Registry, origIn io.Reader, origOut io.Writer) *service.Registry {
	clone := service.Clone(r)
	(&Factory{R: origIn, W: origOut}).Register(clone)
	return clone
}

type origin struct {
	r io.Reader
	w io.Writer
}

func (o *origin) Handle(buf []byte, replies chan<- []byte) {
	if o.w != nil {
		if _, err := o.w.Write(buf[packetHeaderSize:]); err != nil {
			if err == io.EOF {
				o.w = nil
			} else {
				panic(err)
			}
		}
	}
}

func (o *origin) Shutdown() {
}
