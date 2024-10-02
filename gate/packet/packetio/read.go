// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package packetio

import (
	"errors"
	"io"
	"sync/atomic"

	"gate.computer/gate/packet"
	"gate.computer/internal/error/badprogram"
	"import.name/flux"

	. "import.name/type/context"
)

var errNegativeSubscription = badprogram.Error("stream flow increment is negative")

// ReadStream is a unidirectional stream between a reader and a channel.
//
// The channel side calls Subscribe, FinishSubscription, and StopTransfer.  The
// reader side calls Transfer.
//
// State can be unmarshaled before subscription and transfer, and it can be
// marshaled afterwards.
type ReadStream struct {
	State State
	waker flux.Waker
}

// MakeReadStream is useful for initializing a field.  Don't use multiple
// copies of the value.
func MakeReadStream() ReadStream {
	return ReadStream{
		State: InitialState(),
		waker: flux.MakeWaker(),
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

	atomic.AddUint32(&s.State.Subscribed, uint32(increment))
	s.waker.Poke()
	return nil
}

// FinishSubscription signals that no more data will be subscribed to.
func (s *ReadStream) FinishSubscription() error {
	if s.State.Flags&FlagSubscribing == 0 {
		return errors.New("read stream already closed")
	}

	// This is the only Flags storer during the Transfer loop.  The Transfer
	// loop ends either after it sees this mutation or the channel closure;
	// Flags can be read directly here.
	atomic.StoreUint32(&s.State.Flags, s.State.Flags&^FlagSubscribing)
	s.waker.Poke()
	return nil
}

// StopTransfer the transfer.  The subscribe methods must not be called after
// this.
func (s *ReadStream) StopTransfer() {
	s.waker.Finish()
}

// Transfer data from a reader to a service's data stream according to
// subscription.  Buffer size is limited by config.MaxSendSize.
//
// Read or context error is returned, excluding EOF.
func (s *ReadStream) Transfer(ctx Context, config packet.Service, streamID int32, send chan<- packet.Thunk, r io.Reader) error {
	var (
		err     error
		done    = ctx.Done() // Read side cancellation.
		readpos uint32       // Relative to Subscribed.  Wraps around.
		pkt     packet.Buf
	)

	flags := atomic.LoadUint32(&s.State.Flags)
	if flags&FlagReadWriting == 0 {
		closeRead(&r)
	}
	if flags&FlagSendReceiving == 0 {
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

	for send != nil || atomic.LoadUint32(&s.State.Flags)&FlagSubscribing != 0 {
		var sending chan<- packet.Thunk

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

		if sending != nil || r == nil || readpos == atomic.LoadUint32(&s.State.Subscribed) {
			select {
			case sending <- pkt.Thunk():
				pkt = nil
				if r == nil {
					send = nil
				}

			case _, ok := <-s.waker.Chan():
				if !ok {
					goto stopped
				}

			case <-done:
				err = ctx.Err()
				done = nil
				closeRead(&r)
			}

			if sending != nil && pkt != nil {
				continue // Try to send again.
			}
		} else {
			select {
			case _, ok := <-s.waker.Chan():
				if !ok {
					goto stopped
				}

			case <-done:
				err = ctx.Err()
				done = nil
				closeRead(&r)

			default: // No blocking.
			}
		}

		if r != nil {
			if subs := atomic.LoadUint32(&s.State.Subscribed) - readpos; subs != 0 {
				n := config.MaxSendSize - packet.DataHeaderSize
				if uint32(n) > subs {
					n = int(subs)
				}

				b := packet.MakeData(config.Code, streamID, n)
				n, err = r.Read(b.Data())
				pkt, _ = b.Cut(n)
				readpos += uint32(n)
				if err != nil {
					closeRead(&r)
					done = nil
					if err == io.EOF {
						err = nil
					}
				}
			} else if atomic.LoadUint32(&s.State.Flags)&FlagSubscribing == 0 {
				// Subscriber side has requested read stream closure, and more
				// data cannot be read.  Synthesize EOF to acknowledge.
				closeRead(&r)
				done = nil
			}
		}
	}

	// Make sure that Flags and Subscribed have their final modifications by
	// the other side.
	select {
	case <-s.waker.Chan():
	default:
	}

stopped:
	// Update flags tracked by this side.
	flags = s.State.Flags &^ (FlagReadWriting | FlagSendReceiving)
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

func closeRead(r *io.Reader) {
	if *r != nil {
		if c, ok := (*r).(interface{ CloseRead() error }); ok {
			_ = c.CloseRead()
		}
		*r = nil
	}
}

// FinishSubscriber can subscribe to more data and signal that no more data
// will be subscribed to.
type FinishSubscriber interface {
	Subscribe(increment int32) error
	FinishSubscription() error
}

// Subscribe to more data or signal that no more data will be subscribed to.
// The operand must be non-negative.
func Subscribe(s FinishSubscriber, operand int32) error {
	switch {
	case operand > 0:
		return s.Subscribe(operand)

	case operand == 0:
		return s.FinishSubscription()
	}

	panic(operand)
}
