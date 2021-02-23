// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuzzutil

import (
	"context"
	"errors"
	"io"
	goruntime "runtime"
	"time"

	"gate.computer/gate/internal/error/resourcelimit"
	"gate.computer/gate/runtime"
	gateruntime "gate.computer/gate/runtime"
	"gate.computer/gate/server"
	"gate.computer/gate/server/api"
	"gate.computer/gate/service"
	werrors "gate.computer/wag/errors"
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
	e, err := gateruntime.NewExecutor(gateruntime.Config{
		LibDir: libdir,
	})
	if err != nil {
		panic(err)
	}

	services := server.NewInstanceServices(connector{}, new(service.Registry))

	s, err := server.New(context.Background(), &server.Config{
		ProcessFactory: runtime.PrepareProcesses(ctx, e, goruntime.GOMAXPROCS(0)*100),
		AccessPolicy:   server.NewPublicAccess(func(context.Context) server.InstanceServices { return services }),
	})
	if err != nil {
		panic(err)
	}
	return s
}

func IsFine(err error) bool {
	for _, sentinel := range []error{
		io.ErrUnexpectedEOF,
		context.DeadlineExceeded,
	} {
		if errors.Is(err, sentinel) {
			return true
		}
	}

	var moduleError werrors.ModuleError
	var limitError resourcelimit.Error
	if errors.As(err, &moduleError) || errors.As(err, &limitError) {
		return true
	}

	return false
}

func IsGood(s *api.Status) bool {
	if s == nil {
		return false
	}

	switch s.State {
	case api.StateRunning, api.StateSuspended, api.StateHalted, api.StateTerminated, api.StateKilled:
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

	return true
}
