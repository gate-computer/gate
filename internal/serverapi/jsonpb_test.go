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
		State: serverapi.Status_terminated,
		Cause: serverapi.Status_Cause(trap.MemoryAccessOutOfBounds),
	}

	data := serverapi.MustMarshalJSON(status)

	if s := string(data); s != `{"state":"terminated","cause":"memory_access_out_of_bounds"}` {
		t.Error(s)
	}
}
