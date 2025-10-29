// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package packetio

import (
	"context"
	"io"
	"testing"

	"gate.computer/gate/packet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	. "import.name/testing/mustr"
)

func TestWriteStreamEnd(t *testing.T) {
	s := NewWriteStream(512)
	assert.NoError(t, s.CloseWrite())

	r, w := io.Pipe()
	send := make(chan packet.Thunk, 2)

	assert.NoError(t, s.Transfer(context.Background(), testService, testStreamID, w, send))
	assert.False(t, s.Live())

	for {
		thunk := <-send
		p := Must(t, R(thunk()))
		if len(p) > 0 {
			p := packet.MustBeFlow(p)
			require.Equal(t, p.Code(), testService.Code)
			if flowEOF(t, p) {
				break
			}
		}
	}

	w.Close()

	n, err := r.Read(make([]byte, 1))
	assert.Equal(t, err, io.EOF)
	assert.Zero(t, n)
}

func flowEOF(t *testing.T, p packet.FlowBuf) bool {
	var i int

	for i = 0; i < p.Len(); i++ {
		if flow := p.At(i); flow.ID == testStreamID && flow.IsEOF() {
			goto found
		}
	}
	return false

found:
	for i++; i < p.Len(); i++ {
		assert.NotEqual(t, p.At(i).ID, testStreamID)
	}
	return true
}
