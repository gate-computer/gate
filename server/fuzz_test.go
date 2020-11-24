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

	"gate.computer/gate/internal/test/fuzz/fuzzutil"
	"gate.computer/gate/server"
	"gate.computer/gate/server/api"
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

	mod := &server.ModuleUpload{
		Stream: ioutil.NopCloser(bytes.NewReader(data)),
		Length: int64(len(data)),
	}

	launch := &api.LaunchOptions{
		Function:  fuzzutil.Function,
		Transient: true,
	}

	inst, err := s.UploadModuleInstance(ctx, mod, nil, launch, nil)
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
