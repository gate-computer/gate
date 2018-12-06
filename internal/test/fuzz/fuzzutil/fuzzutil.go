// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuzzutil

import (
	"context"
	"io"
	goruntime "runtime"
	"time"

	gateruntime "github.com/tsavola/gate/runtime"
	"github.com/tsavola/gate/server"
	"github.com/tsavola/gate/service"
)

const (
	Function   = "main"
	RunTimeout = 5 * time.Second
)

type connector struct{}

func (connector) Connect(context.Context) func(context.Context, io.Reader, io.Writer) error {
	return nil
}

func (connector) Close() error {
	return nil
}

func NewServer(ctx context.Context, libdir string) *server.Server {
	e, err := gateruntime.NewExecutor(ctx, &gateruntime.Config{
		MaxProcs: goruntime.GOMAXPROCS(0),
		LibDir:   libdir,
	})
	if err != nil {
		panic(err)
	}

	services := server.NewInstanceServices(new(service.Registry), connector{})

	return server.New(ctx, &server.Config{
		Executor:     e,
		AccessPolicy: server.NewPublicAccess(func() server.InstanceServices { return services }),
		PreforkProcs: goruntime.GOMAXPROCS(0) * 100,
	})
}

func IsFine(err error) bool {
	switch err {
	case io.EOF, io.ErrUnexpectedEOF, context.DeadlineExceeded:
		return true
	}

	switch err.(type) {
	case interface{ ModuleError() string }:
		return true
	}

	return false
}
