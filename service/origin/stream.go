// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package origin

import (
	"context"
	"io"

	"github.com/tsavola/gate/packet"
	"github.com/tsavola/gate/packet/packetio"
)

type stream struct {
	packetio.Stream
	state packetio.StreamState // State after tranfer.
	done  chan struct{}        // Closed when state is available.
}

func newStream(bufSize int) *stream {
	return &stream{
		Stream: packetio.MakeStream(bufSize),
		done:   make(chan struct{}),
	}
}

func (s *stream) transfer(ctx context.Context, config packet.Service, streamID int32, r io.Reader, w io.Writer, output chan<- packet.Buf,
) (err error) {
	defer close(s.done)
	s.state, err = s.Transfer(ctx, config, streamID, r, w, output)
	s.state.Write.Discard() // We don't conserve connections.
	return
}
