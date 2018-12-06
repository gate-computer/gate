// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuzz

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"os"
	goruntime "runtime"
	"time"

	gateruntime "github.com/tsavola/gate/runtime"
	"github.com/tsavola/gate/server"
	"github.com/tsavola/gate/service"
)

const (
	function   = "main"
	runTimeout = 5 * time.Second
)

type connector struct{}

func (connector) Connect(context.Context) func(context.Context, io.Reader, io.Writer) error {
	return nil
}

func (connector) Close() error {
	return nil
}

var s *server.Server

func init() {
	ctx := context.Background()

	e, err := gateruntime.NewExecutor(ctx, &gateruntime.Config{
		MaxProcs: goruntime.GOMAXPROCS(0),
		LibDir:   os.Getenv("GATE_FUZZ_RUNTIME_LIBDIR"),
	})
	if err != nil {
		panic(err)
	}

	services := server.NewInstanceServices(new(service.Registry), connector{})

	s = server.New(ctx, &server.Config{
		Executor:     e,
		AccessPolicy: server.NewPublicAccess(func() server.InstanceServices { return services }),
		PreforkProcs: goruntime.GOMAXPROCS(0) * 100,
	})
}

func isFine(err error) bool {
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

func Fuzz(data []byte) int {
	ctx := context.Background()

	inst, err := s.UploadModuleInstance(ctx, nil, "", ioutil.NopCloser(bytes.NewReader(data)), int64(len(data)), function, "")
	if err != nil {
		if isFine(err) {
			return 1
		}
		return 0
	}

	ctx, cancel := context.WithTimeout(ctx, runTimeout)
	defer cancel()

	_, err = inst.Run(ctx, s)
	if err != nil {
		if isFine(err) {
			return 1
		}
		return 0
	}

	return 1
}
