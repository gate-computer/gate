// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package event_test

import (
	"encoding/json"
	"testing"

	"gate.computer/gate/server"
	"gate.computer/gate/server/detail"
	"gate.computer/gate/server/event"
)

func TestFailInternal(t *testing.T) {
	iface := server.AllocateIface("tester")
	if iface != 1 {
		t.Error(iface)
	}

	ev := &event.FailInternal{
		Ctx: detail.Context{
			Iface: iface,
			Req:   123,
			Addr:  "testprogram",
		},
		Subsystem: "testing",
	}

	if s := ev.Ctx.Iface.String(); s != "tester" {
		t.Error(s)
	}

	t.Log(ev)

	data, err := json.Marshal(ev)
	if err != nil {
		t.Error(err)
	}

	if s := string(data); s != `{"ctx":{"iface":1,"req":123,"addr":"testprogram"},"subsystem":"testing"}` {
		t.Error(s)
	}
}
