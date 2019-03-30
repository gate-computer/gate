// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuzz

import (
	"bytes"
	"context"
	"io/ioutil"
	"os"
	"syscall"

	"github.com/tsavola/gate/internal/test/fuzz/fuzzutil"
)

var s = fuzzutil.NewServer(context.Background(), os.Getenv("GATE_FUZZ_RUNTIME_LIBDIR"))

func init() {
	limit := &syscall.Rlimit{
		Cur: 100000,
		Max: 100000,
	}

	if err := setrlimit(syscall.RLIMIT_NOFILE, limit); err != nil {
		panic(err)
	}
}

func Fuzz(data []byte) int {
	ctx, cancel := context.WithTimeout(context.Background(), fuzzutil.RunTimeout)
	defer cancel()

	inst, err := s.UploadModuleInstance(ctx, nil, false, "", ioutil.NopCloser(bytes.NewReader(data)), int64(len(data)), false, fuzzutil.Function, "", "")
	if err != nil {
		if fuzzutil.IsFine(err) {
			return 1
		}
		return 0
	}

	status := inst.Wait(ctx)
	if !fuzzutil.IsGood(status) {
		return 0
	}

	return 1
}
