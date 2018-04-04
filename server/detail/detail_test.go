// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package detail_test

import (
	"encoding/json"
	"testing"

	"github.com/tsavola/gate/server"
	"github.com/tsavola/gate/server/detail"
)

func TestErrorOp(t *testing.T) {
	iface := server.AllocateIface("tester")
	if iface != 1 {
		t.Error(iface)
	}

	op := &detail.Position{
		Context: detail.Context{
			Iface:  iface,
			Client: "testprogram",
		},
		Subsystem: "testing",
	}

	if s := op.Context.Iface.String(); s != "tester" {
		t.Error(s)
	}

	data, err := json.Marshal(op)
	if err != nil {
		t.Error(err)
	}

	if s := string(data); s != `{"context":{"iface":1,"client":"testprogram"},"subsystem":"testing"}` {
		t.Error(s)
	}

	t.Log(op)
}
