package origin

import (
	"io"

	"github.com/tsavola/gate/service"
)

const (
	Name    = "origin"
	Version = 0

	packetHeaderSize = 8

	maxPacketSize = 0x10000 // TODO: move this elsewhere
)

type Factory struct {
	R io.Reader
	W io.Writer
}

func (f *Factory) Register(r *service.Registry) {
	service.Register(r, Name, Version, f)
}

func (f *Factory) New() service.Instance {
	return &origin{r: f.R, w: f.W}
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

	reading chan struct{}
}

func (o *origin) Handle(buf []byte, replies chan<- []byte) {
	if o.r != nil && o.reading == nil {
		o.reading = make(chan struct{})
		go o.readLoop(buf[6:8], replies)
	}

	if o.w != nil {
		if _, err := o.w.Write(buf[packetHeaderSize:]); err != nil {
			// assume that the error is EOF, broken pipe or such
			o.w = nil
		}
	}
}

func (o *origin) Shutdown() {
	if o.reading != nil {
		close(o.reading)
	}
}

func (o *origin) readLoop(code []byte, replies chan<- []byte) {
	for {
		buf := make([]byte, maxPacketSize) // TODO: smaller buffer?
		copy(buf[6:8], code)

		n, err := o.r.Read(buf[packetHeaderSize:])
		if err != nil {
			o.r = nil
			return
		}

		select {
		case replies <- buf[:packetHeaderSize+n]:
			// ok

		case <-o.reading:
			return
		}
	}
}
