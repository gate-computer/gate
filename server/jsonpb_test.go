// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server_test

import (
	"testing"

	"github.com/tsavola/gate/internal/serverapi"
	"github.com/tsavola/gate/server"
	"github.com/tsavola/wag/trap"
)

func TestStatusJSON(t *testing.T) {
	status := &server.Status{
		State: server.StateKilled,
		Cause: server.Cause(trap.MemoryAccessOutOfBounds),
	}

	data := serverapi.MustMarshalJSON(status)

	if s := string(data); s != `{"state":"KILLED","cause":"MEMORY_ACCESS_OUT_OF_BOUNDS"}` {
		t.Error(s)
	}
}
