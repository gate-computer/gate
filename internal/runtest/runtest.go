// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtest

import (
	"os"
	"os/user"
	"strconv"

	"github.com/tsavola/gate/run"
)

func parseId(s string) uint {
	n, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		panic(err)
	}
	return uint(n)
}

type TestRuntime struct {
	*run.Runtime
	Closed bool
}

func (testRT *TestRuntime) Close() error {
	testRT.Closed = true
	return testRT.Runtime.Close()
}

func NewRuntime() (testRT *TestRuntime) {
	containerUser, err := user.Lookup(os.Getenv("GATE_TEST_CONTAINERUSER"))
	if err != nil {
		panic(err)
	}

	executorUser, err := user.Lookup(os.Getenv("GATE_TEST_EXECUTORUSER"))
	if err != nil {
		panic(err)
	}

	config := run.Config{
		ContainerCred: run.Cred{
			Uid: parseId(containerUser.Uid),
			Gid: parseId(containerUser.Gid),
		},
		ExecutorCred: run.Cred{
			Uid: parseId(executorUser.Uid),
			Gid: parseId(executorUser.Gid),
		},
		LibDir: os.Getenv("GATE_TEST_LIBDIR"),
	}

	rt, err := run.NewRuntime(&config)
	if err != nil {
		panic(err)
	}

	testRT = &TestRuntime{Runtime: rt}

	go func() {
		<-testRT.Done()
		if !testRT.Closed {
			panic("executor died")
		}
	}()

	return
}
