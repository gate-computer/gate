// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package origin

import (
	"io"

	"gate.computer/gate/service"

	. "import.name/type/context"
)

const (
	serviceName     = "origin"
	serviceRevision = "0"
)

const (
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
func New(config *Config) *Connector {
	var c Config
	if config != nil {
		c = *config
	}
	if c.MaxConns <= 0 {
		c.MaxConns = DefaultMaxConns
	}
	if c.BufSize <= 0 {
		c.BufSize = DefaultBufSize
	}

	return &Connector{
		inst:   makeInstance(c),
		closed: make(chan struct{}),
	}
}

// Connect allocates a new I/O stream.  The returned function is to be used to
// transfer data between a connection and the program instance.  If it's
// non-nil, a connection was established.
func (cr *Connector) Connect(ctx Context) func(Context, io.Reader, io.WriteCloser) error {
	return cr.inst.connect(ctx, cr.closed)
}

// Close causes currently blocked and future Connect calls to return nil.
// Established connections will not be closed.
func (cr *Connector) Close() error {
	close(cr.closed)
	return nil
}

func (cr *Connector) Properties() service.Properties {
	return service.Properties{
		Service: service.Service{
			Name:     serviceName,
			Revision: serviceRevision,
		},
		Streams: true,
	}
}

func (cr *Connector) Discoverable(Context) bool {
	return true
}

func (cr *Connector) CreateInstance(ctx Context, config service.InstanceConfig, state []byte) (service.Instance, error) {
	cr.inst.init(config.Service)
	if err := cr.inst.restore(state); err != nil {
		return nil, err
	}
	return &cr.inst, nil
}
