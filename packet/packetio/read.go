// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package packetio

import (
	"context"
	"errors"
	"io"
	"sync/atomic"

	"github.com/tsavola/gate/internal/error/badprogram"
	"github.com/tsavola/gate/packet"
)

const errNegativeSubscription = badprogram.Err("stream flow increment is negative")

// ReadStream is a unidirectional stream between a reader and a channel.
//
// The channel side calls Subscribe, SubscribeEOF, and Stop.  The reader side
// calls Transfer.
//
// State can be unmarshaled before subscription and transfer, and it can be
// marshaled afterwards.
type ReadStream struct {
	State  State
	wakeup chan struct{}
}

// MakeReadStream is useful for initializing a field.  Don't use multiple
// copies of the value.
func MakeReadStream() ReadStream {
	return ReadStream{
		State:  InitialState(),
		wakeup: make(chan struct{}, 1),
	}
}

// NewReadStream object.  Use MakeReadStream when embedding ReadStream in a
// struct.
func NewReadStream() *ReadStream {
	s := MakeReadStream()
	return &s
}

// Live state?
//
// The state is undefined during subscription or transfer.
func (s *ReadStream) Live() bool {
	return s.State.Live()
}

// Reading state?  When the stream is no longer in the reading state, reader
// may be specified as nil in Transfer invocation.
//
// The state is undefined during transfer.
func (s *ReadStream) Reading() bool {
	return s.State.Flags&FlagReadWriting != 0
}

// Subscribe to more data.  Causes the concurrent transfer to read and send
// data.
func (s *ReadStream) Subscribe(increment int32) error {
	if s.State.Flags&FlagSubscribing == 0 {
		return errors.New("read stream already closed")
	}

	if increment < 0 {
		return errNegativeSubscription
	}

	// The final wakeup channel poking is matched by at least one wakeup
	// channel receive by Transfer before it loads the final subscription
	// position.
	s.State.Subscribed += uint32(increment)
	poke(s.wakeup)
	return nil
}

// SubscribeEOF signals that no more data will be subscribed to.
func (s *ReadStream) SubscribeEOF() error {
	if s.State.Flags&FlagSubscribing == 0 {
		return errors.New("read stream already closed")
	}

	// This is the only Flags storer during the Transfer loop.  The Transfer
	// loop ends either after it sees this mutation or the channel closure.
	atomic.StoreUint32(&s.State.Flags, s.State.Flags&^FlagSubscribing)
	poke(s.wakeup)
	return nil
}

// Stop the transfer.  The subscribe methods must not be called after this.
func (s *ReadStream) Stop() {
	close(s.wakeup)
}

// Transfer data from a reader to a service's data stream according to
// subscription.  Buffer size is limited by config.MaxSendSize.
//
// Read or context error is returned, excluding EOF.
func (s *ReadStream) Transfer(ctx context.Context, config packet.Service, streamID int32, send chan<- packet.Buf, r io.Reader) error {
	var (
		err     error
		done    = ctx.Done() // Read side cancellation.
		readpos uint32       // Relative to Subscribed.  Wraps around.
		pkt     packet.Buf
	)

	if s.State.Flags&FlagReadWriting == 0 {
		r = nil
	}
	if s.State.Flags&FlagSendReceiving == 0 {
		send = nil
	}
	if send == nil && r != nil {
		panic("reading without sending")
	}

	if len(s.State.Data) != 0 {
		b := packet.MakeData(config.Code, streamID, len(s.State.Data))
		copy(b.Data(), s.State.Data)
		pkt = packet.Buf(b)
		s.State.Data = nil // Don't keep reference to snapshot buffer.
	}

	// The loop accesses Flags and Subscribed fields nonatomically, but wakeup
	// channel reception makes sure that it sees mutations and makes progress.
	for send != nil || s.State.Flags&FlagSubscribing != 0 {
		var sending chan<- packet.Buf

		if send != nil {
			if len(pkt) > packet.DataHeaderSize {
				sending = send
			} else if r == nil {
				if pkt == nil {
					pkt = packet.MakeDataEOF(config.Code, streamID)
				}
				sending = send
			}
		}

		if sending != nil || r == nil || readpos == s.State.Subscribed {
			select {
			case sending <- pkt:
				pkt = nil
				if r == nil {
					send = nil
				}

			case _, ok := <-s.wakeup:
				if !ok {
					goto stopped
				}

			case <-done:
				err = ctx.Err()
				done = nil
				r = nil
			}

			if sending != nil && pkt != nil {
				continue // Try to send again.
			}
		} else {
			select {
			case _, ok := <-s.wakeup:
				if !ok {
					goto stopped
				}

			case <-done:
				err = ctx.Err()
				done = nil
				r = nil

			default: // No blocking.
			}
		}

		if r != nil {
			if subs := s.State.Subscribed - readpos; subs != 0 {
				n := config.MaxSendSize - packet.DataHeaderSize
				if uint32(n) > subs {
					n = int(subs)
				}

				b := packet.MakeData(config.Code, streamID, n)
				n, err = r.Read(b.Data())
				pkt, _ = b.Split(n)
				readpos += uint32(n)
				if err != nil {
					r = nil
					done = nil
					if err == io.EOF {
						err = nil
					}
				}
			} else if s.State.Flags&FlagSubscribing == 0 {
				// Subscriber side has requested read stream closure, and more
				// data cannot be read.  Synthesize EOF to acknowledge.
				r = nil
				done = nil
			}
		}
	}

	// Make sure that the final Flags and Subscribed values are seen.
	select {
	case <-s.wakeup:
	default:
	}

stopped:
	// Update flags tracked by this side.
	flags := s.State.Flags &^ (FlagReadWriting | FlagSendReceiving)
	if r != nil {
		flags |= FlagReadWriting
	}
	if send != nil {
		flags |= FlagSendReceiving
	}
	s.State.Flags = flags

	// Normalize subscription position.
	s.State.Subscribed -= readpos

	// Buffered data.
	if len(pkt) > packet.DataHeaderSize {
		s.State.Data = pkt[packet.DataHeaderSize:]
	}

	return err
}
