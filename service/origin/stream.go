// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package origin

import (
	"context"
	"io"

	"gate.computer/gate/packet"
	"gate.computer/gate/packet/packetio"
)

type stream struct {
	packetio.Stream
	stopped chan struct{}
}

func newStream(bufsize int) *stream {
	return &stream{
		Stream:  packetio.MakeStream(bufsize),
		stopped: make(chan struct{}),
	}
}

func (s *stream) transfer(ctx context.Context, config packet.Service, streamID int32, r io.Reader, w io.Writer, send chan<- packet.Thunk) error {
	defer close(s.stopped)
	err := packetio.RWError(s.Transfer(ctx, config, streamID, r, w, send))
	s.WriteStream.State.Data = nil // Connections are not conserved.
	return err
}
