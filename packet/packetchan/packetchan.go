// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package packetchan

import (
	"context"

	"github.com/tsavola/contextack"
	"github.com/tsavola/gate/packet"
)

type forwardAck struct{}

// ForwardDoneAck can be used with contextack.WithAck to subscribe to Forward
// call cancellation acknowledgement.
var ForwardDoneAck forwardAck

// Forward packets from source to destination until the context is canceled.
func Forward(ctx context.Context, destination chan<- packet.Buf, source <-chan packet.Buf) {
	defer contextack.Ack(ctx, ForwardDoneAck)

	var p packet.Buf

	for {
		var (
			input  <-chan packet.Buf
			output chan<- packet.Buf
			ok     bool
		)

		if p == nil {
			input = source
		} else {
			output = destination
		}

		select {
		case p, ok = <-input:
			if !ok {
				// EOF
				return
			}

		case output <- p:
			// ok

		case <-ctx.Done():
			return
		}
	}
}
