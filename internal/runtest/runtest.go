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

func (testRt *TestRuntime) Close() error {
	testRt.Closed = true
	return testRt.Runtime.Close()
}

func NewRuntime() (testRt *TestRuntime) {
	commonGroup, err := user.LookupGroup(os.Getenv("GATE_TEST_COMMONGROUP"))
	if err != nil {
		panic(err)
	}

	containerUser, err := user.Lookup(os.Getenv("GATE_TEST_CONTAINERUSER"))
	if err != nil {
		panic(err)
	}

	executorUser, err := user.Lookup(os.Getenv("GATE_TEST_EXECUTORUSER"))
	if err != nil {
		panic(err)
	}

	config := run.Config{
		CommonGid: parseId(commonGroup.Gid),
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

	testRt = &TestRuntime{Runtime: rt}

	go func() {
		<-testRt.Done()
		if !testRt.Closed {
			panic("executor died")
		}
	}()

	return
}
