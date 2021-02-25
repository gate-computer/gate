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

func TestWriteStreamEnd(t *testing.T) {
	s := NewWriteStream(512)
	if err := s.CloseWrite(); err != nil {
		t.Error(err)
	}

	r, w := io.Pipe()
	send := make(chan packet.Buf, 2)

	if err := s.Transfer(context.Background(), testService, testStreamID, w, send); err != nil {
		t.Error(err)
	}

	if s.Live() {
		t.Error("still live")
	}

	for {
		p := packet.MustBeFlow(<-send)
		if p.Code() != testService.Code {
			t.Fatal(p)
		}
		if flowEOF(t, p) {
			break
		}
	}

	w.Close()

	if n, err := r.Read(make([]byte, 1)); err != io.EOF || n != 0 {
		t.Error(n, err)
	}
}

func flowEOF(t *testing.T, p packet.FlowBuf) bool {
	var i int

	for i = 0; i < p.Num(); i++ {
		if id, increment := p.Get(i); id == testStreamID && increment == 0 {
			goto found
		}
	}
	return false

found:
	for i++; i < p.Num(); i++ {
		if id, _ := p.Get(i); id == testStreamID {
			t.Fatal(p)
		}
	}
	return true
}
