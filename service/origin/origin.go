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

type Factory struct {
	R io.Reader
	W io.Writer
}

func (f *Factory) Register(r *service.Registry) {
	service.Register(r, Name, Version, f)
}

func (f *Factory) New(code packet.Code, config *service.Config) service.Instance {
	return &origin{
		Factory:     *f,
		code:        code,
		maxReadSize: config.MaxContentSize,
	}
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
	Factory
	code        packet.Code
	maxReadSize int

	reading chan struct{}
}

func (o *origin) Handle(p packet.Buf, replies chan<- packet.Buf) {
	if o.R != nil && o.reading == nil {
		o.reading = make(chan struct{})
		go o.readLoop(replies)
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

func (o *origin) Shutdown() {
	if o.reading != nil {
		close(o.reading)
	}
}

func (o *origin) readLoop(replies chan<- packet.Buf) {
	var buf packet.Buf

	for {
		if buf.ContentSize() < minReadSize {
			buf = packet.Make(o.code, o.maxReadSize)
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
