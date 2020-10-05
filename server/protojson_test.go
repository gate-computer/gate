// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server_test

import (
	"testing"

	"gate.computer/gate/internal/protojson"
	"gate.computer/gate/server/api"
	"github.com/tsavola/wag/trap"
)

func TestStatusJSON(t *testing.T) {
	status := &api.Status{
		State: api.StateKilled,
		Cause: api.Cause(trap.MemoryAccessOutOfBounds),
	}

	data := protojson.MustMarshal(status)

	if s := string(data); s != `{"state":"KILLED","cause":"MEMORY_ACCESS_OUT_OF_BOUNDS"}` {
		t.Error(s)
	}
}
