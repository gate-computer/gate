// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuzzutil

import (
	"context"
	"io"
	goruntime "runtime"
	"time"

	"github.com/tsavola/gate/internal/serverapi"
	"github.com/tsavola/gate/runtime"
	gateruntime "github.com/tsavola/gate/runtime"
	"github.com/tsavola/gate/server"
	"github.com/tsavola/gate/service"
	wagerrors "github.com/tsavola/wag/errors"
	errors "golang.org/x/xerrors"
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
	e, err := gateruntime.NewExecutor(&gateruntime.Config{
		LibDir: libdir,
	})
	if err != nil {
		panic(err)
	}

	services := server.NewInstanceServices(connector{}, new(service.Registry))

	return server.New(&server.Config{
		ProcessFactory: runtime.PrepareProcesses(ctx, e, goruntime.GOMAXPROCS(0)*100),
		AccessPolicy:   server.NewPublicAccess(func(context.Context) server.InstanceServices { return services }),
	})
}

func IsFine(err error) bool {
	for _, sentinel := range []error{
		io.EOF,
		io.ErrUnexpectedEOF,
		context.DeadlineExceeded,
	} {
		if errors.Is(err, sentinel) {
			return true
		}
	}

	var moduleError *wagerrors.ModuleError
	if errors.As(err, &moduleError) {
		return true
	}

	return false
}

func IsGood(s serverapi.Status) bool {
	switch s.State {
	case serverapi.Status_running, serverapi.Status_suspended, serverapi.Status_terminated, serverapi.Status_killed:
	default:
		return false
	}

	switch {
	case s.Cause >= 0 && s.Cause <= 9:
	case s.Cause == 65:
	default:
		return false
	}

	if s.Result != 0 && s.Result != 1 {
		return false
	}

	if s.Error != "" {
		return false
	}

	if s.Debug != "" {
		return false
	}

	return true
}
