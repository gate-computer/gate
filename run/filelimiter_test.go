// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package run_test

import (
	"context"
	"testing"
	"time"

	"github.com/tsavola/gate/internal/runtest"
	"github.com/tsavola/gate/run"
)

func TestFileLimiter(t *testing.T) {
	rt := runtest.NewRuntime(&run.Config{
		FileLimiter: run.NewFileLimiter(11), // Runtime instance holds onto 1 file
	})
	defer rt.Close()

	var (
		images    [10]run.Image
		lastImage run.Image
	)

	ctx := context.Background()

	for i := range images {
		if err := images[i].Init(ctx, rt.Runtime); err != nil {
			t.Fatal(err)
		}
	}

	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		time.Sleep(time.Millisecond)
		cancel()
	}()

	if err := lastImage.Init(cancelCtx, rt.Runtime); err != context.Canceled {
		t.Fatal(err)
	}

	if err := images[0].Release(rt.Runtime); err != nil {
		t.Fatal(err)
	}

	if err := lastImage.Init(ctx, rt.Runtime); err != nil {
		t.Fatal(err)
	}
}
