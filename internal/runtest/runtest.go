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

type TestEnvironment struct {
	*run.Environment
	Closed bool
}

func (testEnv *TestEnvironment) Close() error {
	testEnv.Closed = true
	return testEnv.Environment.Close()
}

func NewEnvironment() (testEnv *TestEnvironment) {
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

	env, err := run.NewEnvironment(&config)
	if err != nil {
		panic(err)
	}

	testEnv = &TestEnvironment{Environment: env}

	go func() {
		<-testEnv.Done()
		if !testEnv.Closed {
			panic("executor died")
		}
	}()

	return
}
