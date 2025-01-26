// Copyright (c) 2020 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package grpc

import (
	"fmt"
	"io"
	"log/slog"
	"strings"

	"gate.computer/gate/service"
	"gate.computer/grpc/client"
	"gate.computer/grpc/executable"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	. "import.name/type/context"
)

type conn interface {
	Register(*service.Registry) error
	io.Closer
}

// Config for gRPC services.
type Config struct {
	// Commands are space-delimited arguments for executing a program.  The
	// arguments may be prefixed with @path if it differs from argv[0].
	Commands []string

	// Target addresses for gRPC connections.  The address may be followed by
	// space-delimited dial options.  Supported options:
	//
	//     "insecure" - no encryption or authentication
	//     "optional" - ignores target on connection error
	//
	Targets []string

	conns []conn
}

// Init configured services.
func (conf *Config) Init(ctx Context, log *slog.Logger) error {
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

		c, err := executable.Execute(ctx, path, args, log.With("program", args[0]))
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
				opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))

			case "optional":
				optional = true

			default:
				return fmt.Errorf("unknown dial option in gRPC target configuration: %q", s)
			}
		}

		c, err := client.NewClient(ctx, args[0], opts...)
		if err != nil {
			if optional {
				log.InfoContext(ctx, "optional connection failed", "addr", args[0], "error", err)
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
func (conf *Config) Register(r *service.Registry) error {
	for _, c := range conf.conns {
		if err := c.Register(r); err != nil {
			return err
		}
	}
	return nil
}

// Close initialized services.
func (conf *Config) Close() (err error) {
	for _, c := range conf.conns {
		if e := c.Close(); err == nil {
			err = e
		}
	}
	conf.conns = nil
	return
}
