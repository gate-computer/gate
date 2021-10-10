// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package packetio

import (
	"context"
	"io"
	"testing"

	"gate.computer/gate/packet"
)

func TestReadStreamEnd(t *testing.T) {
	s := NewReadStream()
	if err := s.FinishSubscription(); err != nil {
		t.Error(err)
	}

	send := make(chan packet.Thunk, 1)
	r, w := io.Pipe()
	w.Close()

	if err := s.Transfer(context.Background(), testService, testStreamID, send, r); err != nil {
		t.Error(err)
	}

	if s.Live() {
		t.Error("still live")
	}

	thunk := <-send
	if p := thunk(); len(p) > 0 {
		p := packet.MustBeData(p)
		if p.Code() != testService.Code || p.ID() != testStreamID {
			t.Error(p)
		}
	}
}
