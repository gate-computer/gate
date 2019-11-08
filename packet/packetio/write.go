// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package packetio

import (
	"context"
	"errors"
	"io"
	"math/bits"
	"sync/atomic"

	"github.com/tsavola/gate/internal/error/badprogram"
	"github.com/tsavola/gate/packet"
)

const errWriteBufferOverflow = badprogram.Err("write stream buffer overflow")

// WriteStream is a unidirectional stream between a channel and a writer.
//
// The channel side calls Write, WriteEOF, and Stop.  The writer side calls
// Transfer.
//
// State can be unmarshaled before writing and transfer, and it can be
// marshaled afterwards.
type WriteStream struct {
	State    State
	wakeup   chan struct{}
	produced uint32 // Circular data buffer offsets.  Must be wrapped to buffer
	consumed uint32 // size at read time (if necessary).
}

// MakeWriteStream is useful for initializing a field.  Don't use multiple
// copies of the value.
//
// Buffer size must be a power of two.
func MakeWriteStream(bufferSize int) WriteStream {
	if bits.OnesCount32(uint32(bufferSize)) != 1 { // Must be a power of two.
		panic("invalid buffer size")
	}

	return WriteStream{
		State:  InitialStateWithDataBuffer(make([]byte, 0, bufferSize)),
		wakeup: make(chan struct{}, 1),
	}
}

// NewWriteStream object.  Use MakeWriteStream when embedding WriteStream in a
// struct.
//
// Buffer size must be a power of two.
func NewWriteStream(bufferSize int) *WriteStream {
	s := MakeWriteStream(bufferSize)
	return &s
}

func (s *WriteStream) bufferSize() int {
	return cap(s.State.Data)
}

// Live state?
//
// The state is undefined during writing or transfer.
func (s *WriteStream) Live() bool {
	return s.State.Live()
}

// Writing state?  When the stream is no longer in the writing state, writer
// may be specified as nil in Transfer invocation.
//
// The state is undefined during transfer.
func (s *WriteStream) Writing() bool {
	return s.State.Flags&FlagReadWriting != 0
}

// Write all data or return an error.
func (s *WriteStream) Write(data []byte) (n int, err error) {
	var (
		size = s.bufferSize()
		mask = uint32(size) - 1
	)

	if s.State.Flags&FlagSendReceiving == 0 {
		err = errors.New("write stream already closed")
		return
	}

	used := s.produced - atomic.LoadUint32(&s.consumed)
	if uint64(used)+uint64(len(data)) >= uint64(size) { // Leave a one-byte gap.
		err = errWriteBufferOverflow
		return
	}

	off := s.produced & mask
	n = copy(s.State.Data[off:size], data)
	if tail := data[n:]; len(tail) > 0 {
		n += copy(s.State.Data[:size], tail)
	}

	s.produced += uint32(n) // TODO: XXX: atomic?
	poke(s.wakeup)
	return
}

// WriteEOF signals that no more data will be written.
func (s *WriteStream) WriteEOF() error {
	if s.State.Flags&FlagSendReceiving == 0 {
		return errors.New("write stream already closed")
	}

	atomic.StoreUint32(&s.State.Flags, s.State.Flags&^FlagSendReceiving)
	poke(s.wakeup)
	return nil
}

// Stop the transfer.  The write methods must not be called after this.
func (s *WriteStream) Stop() {
	close(s.wakeup)
}

// Transfer data from a service's data stream while managing its flow.
//
// Write or context error is returned, excluding EOF.
func (s *WriteStream) Transfer(ctx context.Context, config packet.Service, streamID int32, w io.Writer, send chan<- packet.Buf) error {
	var (
		err  error
		done = ctx.Done()
		size = uint32(s.bufferSize())
		mask = size - 1
		pkt  packet.FlowBuf
	)

	if s.State.Flags&FlagReadWriting == 0 {
		w = nil
	}
	if s.State.Flags&FlagSubscribing == 0 {
		send = nil
	}

	if s.State.Subscribed >= size { // One byte must have been left unsubscribed.
		return errors.New("subscription overflow")
	}

	for send != nil || s.State.Flags&FlagSendReceiving != 0 {
		var (
			increment int32
			sending   chan<- packet.Buf
		)

		if send != nil {
			if w != nil {
				increment = int32(size-s.State.Subscribed) - 1 // Leave a one-byte gap.
			}
			if w == nil || increment > 0 {
				if pkt == nil {
					pkt = packet.MakeFlows(config.Code, 1)
				}
				pkt.Set(0, streamID, increment)
				sending = send
			}
		}

		var consumed = s.consumed & mask

		if w == nil || consumed == s.produced&mask {
			select {
			case sending <- packet.Buf(pkt):
				s.State.Subscribed += uint32(increment)
				pkt = nil
				if w == nil {
					send = nil
				}

			case _, ok := <-s.wakeup:
				if !ok {
					goto stopped
				}

			case <-done:
				err = ctx.Err()
				done = nil
				w = nil
			}
		} else {
			select {
			case sending <- packet.Buf(pkt):
				s.State.Subscribed += uint32(increment)
				pkt = nil
				if w == nil {
					send = nil
				}

			case _, ok := <-s.wakeup:
				if !ok {
					goto stopped
				}

			case <-done:
				err = ctx.Err()
				done = nil
				w = nil

			default: // No blocking.
			}
		}

		if w != nil {
			if produced := s.produced & mask; consumed != produced {
				var b []byte

				if consumed < produced {
					b = s.State.Data[consumed:produced]
				} else {
					b = s.State.Data[consumed:size] // Just the first part this time.
				}

				var n int

				n, err = w.Write(b)
				atomic.StoreUint32(&s.consumed, s.consumed+uint32(n))
				s.State.Subscribed -= uint32(n)
				if err != nil {
					w = nil
					done = nil
					if err == io.EOF {
						err = nil
					}
				}
			} else if s.State.Flags&FlagSendReceiving == 0 {
				// Writer side has requested write stream closure, and there is
				// no more data to write.  Causes subscription to be ended.
				w = nil
				done = nil
			}
		}
	}

	// Make sure that the final Flags value is seen.
	select {
	case <-s.wakeup:
	default:
	}

stopped:
	// Update flags tracked by this side.
	flags := s.State.Flags &^ (FlagReadWriting | FlagSubscribing)
	if w != nil {
		flags |= FlagReadWriting
	}
	if send != nil {
		flags |= FlagSubscribing
	}
	s.State.Flags = flags

	// Convert the circular buffer to a linear buffer.
	consumed := s.consumed & mask
	produced := s.produced & mask
	switch {
	case consumed == produced:
		s.State.Data = nil
	case consumed < produced:
		s.State.Data = s.State.Data[consumed:produced]
	case consumed > produced:
		b := make([]byte, produced-consumed)
		n := copy(b, s.State.Data[consumed:size])
		copy(b[n:], s.State.Data[:produced])
		s.State.Data = b
	}

	return err
}
