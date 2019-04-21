// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package packetio

import (
	"context"
	"io"
	"math"

	"github.com/tsavola/gate/packet"
	"github.com/tsavola/reach/cover"
)

// ReadTo a service's data stream according to its flow.  A previously buffered
// data packet may be supplied.  Buffer size is limited by
// config.MaxPacketSize.  nil reader acts like a closed connection.  An EOF
// packet will NOT be sent automatically.
//
// Buffered state is returned if context is done.  Any read error is returned
// (including EOF).  Context error is not returned.
func ReadTo(ctx context.Context, config packet.Service, streamID int32, output chan<- packet.Buf, bufpacket packet.DataBuf, flow *Threshold, r io.Reader,
) (ReadState, error) {
	var (
		wakeup   = flow.Changed()
		limit    = flow.Current()
		read     uint32
		buflimit int
	)

	if len(bufpacket) != 0 {
		buflimit = len(bufpacket) - packet.DataHeaderSize
		read = uint32(buflimit)
	} else {
		bufpacket = nil
	}

	var err error
	if cover.Bool(r == nil) {
		err = io.EOF
	}

loop:
	for {
		sendable := (len(bufpacket) > packet.DataHeaderSize)
		if !sendable {
			if r == nil {
				break
			}
			if wakeup == nil && read == limit {
				break
			}
		}

		if sendable || read == limit {
			var outC chan<- packet.Buf
			var outP packet.Buf

			if cover.Bool(sendable) {
				outP = packet.Buf(bufpacket)
				outC = output
			}

			select {
			case outC <- outP:
				bufpacket = nil

			case _, ok := <-wakeup:
				if cover.Bool(!ok) {
					wakeup = nil
					break
				}

			case <-ctx.Done():
				break loop
			}
		}

		if cover.Bool(r != nil) {
			limit = flow.Current()

			if recv := cover.MinMaxUint32(limit-read, 0, math.MaxInt32); recv != 0 {
				off := len(bufpacket)
				cover.Cond(off == 0, off == packet.DataHeaderSize, off > packet.DataHeaderSize)
				if off == 0 {
					off = packet.DataHeaderSize
				}

				if space := config.MaxPacketSize - off; cover.Bool(recv > uint32(space)) {
					recv = uint32(space)
				}

				size := off + int(recv)

				switch {
				case bufpacket == nil:
					if cover.Bool(int(recv) > buflimit) {
						buflimit = int(recv)
					}
					bufpacket = packet.MakeData(config.Code, streamID, buflimit)[:packet.DataHeaderSize]

				case cover.Bool(cap(bufpacket) < size):
					cover.Bool(len(bufpacket) == cap(bufpacket))

					b := make(packet.DataBuf, off, size)
					copy(b, bufpacket)
					bufpacket = b
				}

				var n int
				n, err = r.Read(bufpacket[off:size])
				bufpacket = bufpacket[:off+n]
				read += uint32(cover.MinMax(n, 0, size-off))
				if cover.Error(err) != nil {
					r = nil
				}
			}
		}
	}

	if cover.Bool(len(bufpacket) == packet.DataHeaderSize) {
		bufpacket = nil
	}

	suspended := ReadState{
		Buffer:     bufpacket,
		Subscribed: int32(limit - read),
	}
	cover.Min(len(suspended.Buffer), 0)
	cover.MinMaxInt32(suspended.Subscribed, 0, math.MaxInt32)

	return suspended, cover.Error(err)
}
