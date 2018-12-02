// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package serverapi_test

import (
	"testing"

	"github.com/tsavola/gate/internal/serverapi"
	"github.com/tsavola/wag/trap"
)

func TestStatusJSON(t *testing.T) {
	status := &serverapi.Status{
		State: serverapi.Status_TERMINATED,
		Cause: serverapi.Status_VIOLATION,
		Trap:  serverapi.Status_TrapId(trap.MemoryAccessOutOfBounds),
	}

	t.Log(status)

	data, err := serverapi.MarshalJSON(status)
	if err != nil {
		t.Error(err)
	}

	if s := string(data); s != `{"state":"terminated","cause":"violation","trap":"memory_access_out_of_bounds"}` {
		t.Error(s)
	}
}
