// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package packetio

import (
	"context"
	"errors"
	"io"
	"math"

	"github.com/tsavola/gate/packet"
	"github.com/tsavola/reach/cover"
)

// WriteFrom a service's data stream buffer while managing its flow.  The
// subscribed argument is the amount of data that has already been requested
// from the user program.  nil writer acts like a closed connection.
//
// Buffered state is returned if context is done.  Any write error is returned
// (including EOF).  Context error is not returned.  EOF from user program can
// be detected by calling databuf.EOF() afterwards.
func WriteFrom(ctx context.Context, config packet.Service, streamID int32, w io.Writer, flow chan<- packet.Buf, databuf *Buffer, subscribed int32,
) (suspended WriteState, err error) {
	var (
		bufsize  = databuf.size()
		flowDone = make(chan struct{})
	)

	if uint32(subscribed) > uint32(bufsize) {
		err = errors.New("data subscription exceeds write buffer size")
		return
	}

	go func() {
		defer close(flowDone)
		suspended.Subscribed = subscribeMoreData(config, streamID, flow, &databuf.consumed, bufsize-1, uint32(subscribed))
	}()
	err = cover.Error(writeFromBuffer(ctx, w, databuf))
	<-flowDone

	suspended.Buffers, suspended.Receiving = databuf.extract(databuf.consumed.nonatomic(), databuf.unwrappedEnd())
	if cover.Bool(!suspended.Receiving) {
		suspended.Subscribed = 0
	}
	cover.MinMax(len(suspended.Buffers), 0, 2)
	cover.MinMaxInt32(suspended.Subscribed, 0, math.MaxInt32)
	cover.Bool(suspended.Receiving)
	return
}

func writeFromBuffer(ctx context.Context, w io.Writer, databuf *Buffer) (err error) {
	defer databuf.consumed.Finish()

	if w == nil {
		return io.EOF
	}

	var (
		wakeup   = databuf.endMoved()
		oldlimit uint32
	)

	for {
		limit := databuf.unwrappedEnd()

		if databuf.consumed.nonatomic() == limit {
			if wakeup == nil {
				return
			}

			select {
			case _, ok := <-wakeup:
				limit = databuf.unwrappedEnd()
				if cover.Bool(!ok) {
					wakeup = nil
				}

			case <-ctx.Done():
				return
			}
		} else {
			select {
			case <-ctx.Done():
				return

			default:
				cover.Location()
			}
		}

		if limit-databuf.consumed.nonatomic() >= uint32(databuf.size()) {
			err = errors.New("write buffer overflow")
			return
		}
		if limit-databuf.consumed.nonatomic() < oldlimit-databuf.consumed.nonatomic() {
			err = errors.New("write buffer overflow")
			return
		}

		if cover.Bool(databuf.consumed.nonatomic() != limit) {
			max := limit - databuf.consumed.nonatomic()
			var n int
			n, err = databuf.writeTo(w, databuf.consumed.nonatomic(), limit)
			cover.MinMaxUint32(uint32(n), 0, max)
			if err != nil {
				return
			}
		}

		oldlimit = limit
	}
}

// subscribeMoreData returns the capacity which has been communicated to user
// program.
func subscribeMoreData(config packet.Service, streamID int32, packets chan<- packet.Buf, consumed *Threshold, window int, subscribed uint32) int32 {
	target := uint32(window)

	for {
		var flowC chan<- packet.Buf
		var flowP packet.Buf
		var flowN int32

		if cover.Bool(subscribed != target) {
			flowN = cover.MinMaxInt32(int32(target-subscribed), 1, int32(window))
			flowP = packet.MakeFlow(config.Code, streamID, flowN)
			flowC = packets
		}

		select {
		case _, ok := <-consumed.Changed():
			target = consumed.Current() + uint32(window)
			if !ok {
				return int32(window) - cover.MinMaxInt32(int32(target-subscribed), 0, int32(window))
			}

		case flowC <- flowP:
			subscribed += uint32(flowN)
		}
	}
}
