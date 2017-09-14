// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package run_test

import (
	"context"
	"fmt"
	"net"
	"os"
	"syscall"
	"testing"

	"github.com/tsavola/gate/internal/runtest"
	"github.com/tsavola/gate/run"
)

func TestRuntimeEnvironmentChecksumSame(t *testing.T) {
	rt1 := runtest.NewRuntime(nil)
	defer rt1.Close()

	rt2 := runtest.NewRuntime(nil)
	defer rt2.Close()

	if rt1.EnvironmentChecksum != rt2.EnvironmentChecksum {
		t.Fail()
	}
}

func TestRuntimeEnvironmentChecksumDifferent(t *testing.T) {
	// Have to use DaemonSocket with custom LibDir because there is no
	// gate-container binary in the testdata directory.

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addr := fmt.Sprintf("testdata/%d.sock", syscall.Getpid())
	defer os.Remove(addr)

	l, err := net.Listen("unix", addr)
	if err != nil {
		panic(err)
	}

	go func() {
		defer l.Close()

		conn, err := l.Accept()
		if err != nil {
			panic(err)
		}
		defer conn.Close()

		<-ctx.Done()
	}()

	rt1 := runtest.NewRuntime(nil)
	defer rt1.Close()

	rt2, err := run.NewRuntime(ctx, &run.Config{
		DaemonSocket: addr,
		LibDir:       "testdata",
	})
	if err != nil {
		panic(err)
	}
	defer rt2.Close()

	if rt1.EnvironmentChecksum == rt2.EnvironmentChecksum {
		t.Fail()
	}

	cancel()
}
