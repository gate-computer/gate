// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server_test

import (
	"bytes"
	"context"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/tsavola/gate/internal/test/fuzz/fuzzutil"
	"github.com/tsavola/gate/server"
)

func TestFuzz(t *testing.T) {
	dir := os.Getenv("GATE_TEST_FUZZ")
	if dir == "" {
		t.Skip("GATE_TEST_FUZZ directory not set")
	}

	ctx := context.Background()
	s := fuzzutil.NewServer(ctx, "../lib/gate/runtime")

	infos, err := ioutil.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	var tested bool

	for _, info := range infos {
		if !strings.Contains(info.Name(), ".") {
			filename := path.Join(dir, info.Name())

			t.Run(info.Name(), func(t *testing.T) {
				if testing.Verbose() {
					println(filename)
				} else {
					t.Parallel()
				}

				fuzzTest(ctx, t, s, filename)
			})

			tested = true
		}
	}

	if !tested {
		t.Logf("%s does not contain any samples", dir)
	}
}

func fuzzTest(ctx context.Context, t *testing.T, s *server.Server, filename string) {
	t.Helper()

	var ok bool
	defer func() {
		if !ok {
			t.Error(t.Name())
		}
	}()

	data, err := ioutil.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(ctx, fuzzutil.RunTimeout)
	defer cancel()

	inst, err := s.UploadModuleInstance(ctx, false, "", ioutil.NopCloser(bytes.NewReader(data)), int64(len(data)), false, fuzzutil.Function, "", "")
	if err != nil {
		if fuzzutil.IsFine(err) {
			t.Log(err)
		} else {
			t.Fatal(err)
		}
	}

	status := inst.Wait(ctx)
	ok = fuzzutil.IsGood(status)
}
