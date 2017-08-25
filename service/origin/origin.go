// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package origin

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"

	"github.com/tsavola/contextack"
	"github.com/tsavola/gate/packet"
	"github.com/tsavola/gate/packet/packetchan"
	"github.com/tsavola/gate/service"
)

const (
	Name    = "origin"
	Version = 0

	minReadSize = 1536
)

type Origin struct {
	R io.Reader
	W io.Writer
}

var Default = &Origin{
	R: bytes.NewReader(nil),
	W: ioutil.Discard,
}

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
	shutdown    <-chan struct{}
}

func (i *instance) Handle(ctx context.Context, p packet.Buf, replies chan<- packet.Buf) {
	if i.shutdown == nil {
		i.init(ctx, p.Code(), replies)
	}

	if content := p.Content(); len(content) > 0 {
		if _, err := i.W.Write(content); err != nil {
			// assume that the error is EOF, broken pipe or such
			i.W = ioutil.Discard
		}
	}
}

func (i *instance) init(ctx context.Context, code packet.Code, replies chan<- packet.Buf) {
	reads := make(chan packet.Buf)
	go i.readLoop(ctx, code, reads)
	ctx, i.shutdown = contextack.WithAck(ctx, packetchan.ForwardDoneAck)
	go packetchan.Forward(ctx, replies, reads)
}

func (i *instance) Shutdown() {
	if i.shutdown != nil {
		<-i.shutdown
	}
}

func (i *instance) readLoop(ctx context.Context, code packet.Code, reads chan<- packet.Buf) {
	defer close(reads)

	var buf packet.Buf

	for {
		if buf.ContentSize() < minReadSize {
			buf = packet.Make(code, i.maxReadSize)
		}

		n, err := i.R.Read(buf.Content())
		if err != nil {
			return
		}

		var p packet.Buf
		p, buf = buf.Split(n)

		select {
		case reads <- p:
			// ok

		case <-ctx.Done():
			return
		}
	}
}
