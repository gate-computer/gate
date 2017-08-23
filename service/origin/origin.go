// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package origin

import (
	"io"

	"github.com/tsavola/gate/packet"
	"github.com/tsavola/gate/service"
)

const (
	Name    = "origin"
	Version = 0

	minReadSize = 4096
)

type Origin struct {
	R io.Reader
	W io.Writer
}

var Default = new(Origin)

func New(r io.Reader, w io.Writer) *Origin {
	return &Origin{r, w}
}

func (o *Origin) Register(r *service.Registry) {
	r.Register(Name, Version, o)
}

func (o *Origin) Instantiate(code packet.Code, config *service.Config) service.Instance {
	return &instance{
		Origin:      *o,
		maxReadSize: config.MaxContentSize,
	}
}

type instance struct {
	Origin
	maxReadSize int

	reading chan struct{}
}

func (o *instance) Handle(p packet.Buf, replies chan<- packet.Buf) {
	if o.R != nil && o.reading == nil {
		o.reading = make(chan struct{})
		go o.readLoop(p.Code(), replies)
	}

	if o.W != nil {
		if content := p.Content(); len(content) > 0 {
			if _, err := o.W.Write(content); err != nil {
				// assume that the error is EOF, broken pipe or such
				o.W = nil
			}
		}
	}
}

func (o *instance) Shutdown() {
	if o.reading != nil {
		close(o.reading)
	}
}

func (o *instance) readLoop(code packet.Code, replies chan<- packet.Buf) {
	var buf packet.Buf

	for {
		if buf.ContentSize() < minReadSize {
			buf = packet.Make(code, o.maxReadSize)
		}

		n, err := o.R.Read(buf.Content())
		if err != nil {
			return
		}

		var p packet.Buf
		p, buf = buf.Split(n)

		select {
		case replies <- p:
			// ok

		case <-o.reading:
			return
		}
	}
}
