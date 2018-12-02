// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtimeutil

import (
	"context"
	"io/ioutil"

	"github.com/tsavola/gate/runtime"
)

func MustReadFile(filename string) (data []byte) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		panic(err)
	}
	return
}

type Executor struct {
	*runtime.Executor
	Closed bool
}

func (test *Executor) Close() error {
	test.Closed = true
	return test.Executor.Close()
}

func NewExecutor(ctx context.Context, config *runtime.Config) (test *Executor) {
	e, err := runtime.NewExecutor(ctx, config)
	if err != nil {
		panic(err)
	}

	test = &Executor{Executor: e}

	go func() {
		<-test.Dead()
		if !test.Closed {
			panic("executor died")
		}
	}()

	return
}
