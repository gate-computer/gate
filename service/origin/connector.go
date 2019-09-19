// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package origin

import (
	"context"
	"io"

	"github.com/tsavola/gate/service"
)

const (
	ServiceName    = "origin"
	ServiceVersion = "0"

	DefaultMaxConns = 3
	DefaultBufSize  = 32768
)

type Config struct {
	MaxConns int
	BufSize  int
}

var DefaultConfig = Config{
	MaxConns: DefaultMaxConns,
	BufSize:  DefaultBufSize,
}

type Connector struct {
	inst   instance
	closed chan struct{}
}

// New Connector instance for serving one (and only one) program instance.
func New(config Config) *Connector {
	if config.MaxConns <= 0 {
		config.MaxConns = DefaultMaxConns
	}
	if config.BufSize <= 0 {
		config.BufSize = DefaultBufSize
	}

	return &Connector{
		inst:   makeInstance(config),
		closed: make(chan struct{}),
	}
}

// Connect allocates a new I/O stream.  The returned function is to be used to
// transfer data between a connection and the program instance.  If it's
// non-nil, a connection was established.
func (cr *Connector) Connect(ctx context.Context) func(context.Context, io.Reader, io.Writer) error {
	return cr.inst.connect(ctx, cr.closed)
}

// Close causes currently blocked and future Connect calls to return nil.
// Established connections will not be closed.
func (cr *Connector) Close() (err error) {
	close(cr.closed)
	return
}

func (*Connector) ServiceName() string               { return ServiceName }
func (*Connector) ServiceVersion() string            { return ServiceVersion }
func (*Connector) Discoverable(context.Context) bool { return true }

func (cr *Connector) CreateInstance(ctx context.Context, config service.InstanceConfig,
) service.Instance {
	cr.inst.init(config.Service)
	return &cr.inst
}

func (cr *Connector) RestoreInstance(ctx context.Context, config service.InstanceConfig, state []byte,
) (service.Instance, error) {
	cr.inst.init(config.Service)
	if err := cr.inst.restore(state); err != nil {
		return nil, err
	}

	return &cr.inst, nil
}
