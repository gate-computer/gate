// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build amd64

package run_test

import (
	"testing"

	"github.com/tsavola/gate/internal/runtest"
)

// Update this when making a change to the runtime on purpose.
const runtimeEnvironmentChecksum = 0x1ded1b0deed12a0d

func TestRuntimeEnvironmentChecksumUnchanged(t *testing.T) {
	rt := runtest.NewRuntime(nil)
	defer rt.Close()

	if rt.EnvironmentChecksum != runtimeEnvironmentChecksum {
		t.Errorf("0x%016x", rt.EnvironmentChecksum)
	}
}
