// Copyright (c) 2020 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package config

import (
	"context"
	"fmt"
	"io"
	"strings"

	"gate.computer/gate/service"
	grpcservice "gate.computer/gate/service/grpc"
	"gate.computer/gate/service/grpc/executable"
	"google.golang.org/grpc"
)

// Config for global gRPC services.
var Config = new(Conf)

type Logger = executable.Logger

// InitServices per global configuration and register them.
func InitServices(ctx context.Context, r *service.Registry, stderr Logger) error {
	if err := Config.Init(ctx, stderr); err != nil {
		return err
	}
	return Config.Register(r)
}

type conn interface {
	Register(*service.Registry) error
	io.Closer
}

// Conf for gRPC services.
type Conf struct {
	// Commands are space-delimited arguments for executing a program.  The
	// arguments may be prefixed with @path if it differs from argv[0].
	Commands []string

	// Target addresses for remote gRPC connections.  The address may be
	// followed by space-delimited dial options.  Supported options:
	// "insecure".
	Targets []string

	conns []conn
}

// Init configured services.
func (conf *Conf) Init(ctx context.Context, stderr Logger) error {
	var conns []conn
	defer func() {
		for _, c := range conns {
			c.Close()
		}
	}()

	for _, command := range conf.Commands {
		args := strings.Fields(command)
		path := args[0]
		if path[0] == '@' {
			path = path[1:]
			args = args[1:]
		}

		c, err := executable.Execute(ctx, path, args, stderr)
		if err != nil {
			return err
		}

		conns = append(conns, c)
	}

	for _, target := range conf.Targets {
		args := strings.Fields(target)

		var (
			opts     []grpc.DialOption
			optional bool
		)
		for _, s := range args[1:] {
			switch s {
			case "insecure":
				opts = append(opts, grpc.WithInsecure())

			case "optional":
				optional = true

			default:
				return fmt.Errorf("unknown dial option in gRPC target configuration: %q", s)
			}
		}

		c, err := grpcservice.DialContext(ctx, args[0], opts...)
		if err != nil {
			if optional {
				stderr.Printf("%v", err)
				continue
			}
			return err
		}

		conns = append(conns, c)
	}

	conf.conns = conns
	conns = nil
	return nil
}

// Register initialized services.
func (conf *Conf) Register(r *service.Registry) error {
	for _, c := range conf.conns {
		if err := c.Register(r); err != nil {
			return err
		}
	}
	return nil
}

// Close initialized services.
func (conf *Conf) Close() (err error) {
	for _, c := range conf.conns {
		if e := c.Close(); err == nil {
			err = e
		}
	}
	conf.conns = nil
	return
}
