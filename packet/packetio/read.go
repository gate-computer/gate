// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package packetio

import (
	"context"
	"io"

	"github.com/tsavola/gate/packet"
)

// ReadTo a service's data stream according to its flow.  A previously buffered
// data packet may be supplied.  Buffer size is limited by config.MaxSendSize.
// nil reader acts like a closed connection.  An EOF packet will NOT be sent
// automatically.
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
	if r == nil {
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

			if sendable {
				outP = packet.Buf(bufpacket)
				outC = output
			}

			select {
			case outC <- outP:
				bufpacket = nil

			case _, ok := <-wakeup:
				if !ok {
					wakeup = nil
					break
				}

			case <-ctx.Done():
				break loop
			}
		}

		if r != nil {
			limit = flow.Current()

			if recv := limit - read; recv != 0 {
				off := len(bufpacket)
				if off == 0 {
					off = packet.DataHeaderSize
				}

				if space := config.MaxSendSize - off; recv > uint32(space) {
					recv = uint32(space)
				}

				size := off + int(recv)

				switch {
				case bufpacket == nil:
					if int(recv) > buflimit {
						buflimit = int(recv)
					}
					bufpacket = packet.MakeData(config.Code, streamID, buflimit)[:packet.DataHeaderSize]

				case cap(bufpacket) < size:
					b := make(packet.DataBuf, off, size)
					copy(b, bufpacket)
					bufpacket = b
				}

				var n int
				n, err = r.Read(bufpacket[off:size])
				bufpacket = bufpacket[:off+n]
				read += uint32(n)
				if err != nil {
					r = nil
				}
			}
		}
	}

	if len(bufpacket) == packet.DataHeaderSize {
		bufpacket = nil
	}

	suspended := ReadState{
		Buffer:     bufpacket,
		Subscribed: int32(limit - read),
	}

	return suspended, err
}
