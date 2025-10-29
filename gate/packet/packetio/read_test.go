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

	. "import.name/testing/mustr"
)

func TestReadStreamEnd(t *testing.T) {
	s := NewReadStream()
	assert.NoError(t, s.FinishSubscription())

	send := make(chan packet.Thunk, 1)
	r, w := io.Pipe()
	w.Close()

	assert.NoError(t, s.Transfer(context.Background(), testService, testStreamID, send, r))
	assert.False(t, s.Live())

	thunk := <-send
	p := Must(t, R(thunk()))
	if len(p) > 0 {
		p := packet.MustBeData(p)
		assert.Equal(t, p.Code(), testService.Code)
		assert.Equal(t, p.ID(), testStreamID)
	}
}
