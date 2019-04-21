// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package packetio provides streaming utilities.
//
// Streamer and ReStreamer are high-level tools for implementing bi-directional
// streams.  They build on the ReadTo, WriteFrom, Buffer and Threshold
// primitives.
//
// StreamState, ReadState and WriteState implement simple binary serialization
// of suspended I/O state.
package packetio

import (
	"context"
	"errors"
	"io"
	"math"

	"github.com/tsavola/gate/packet"
	"github.com/tsavola/reach/cover"
)

var errPanicWrite = errors.New("panic while writing")

func isFailure(err error) bool {
	cover.EOF(err)
	return err != nil && err != io.EOF
}

// StreamTerminal is the user program side of a stream.
type StreamTerminal struct {
	Buffer   // Write buffer.
	ReadFlow Threshold

	readBuffer      packet.DataBuf
	writeSubscribed int32
}

func makeStreamTerminal(bufsize int) StreamTerminal {
	return StreamTerminal{
		Buffer:   MakeBuffer(bufsize),
		ReadFlow: MakeThreshold(),
	}
}

// Finish reading and writing.  WriteEOF should be called before this, if
// applicable.
func (s *StreamTerminal) Finish() {
	s.Buffer.Finish()
	s.ReadFlow.Finish()
}

// Streamer is used for bi-directional streaming.
type Streamer struct {
	StreamTerminal
}

// MakeStreamer is for initializing a field.  The value must not be copied.
func MakeStreamer(bufsize int) Streamer {
	return Streamer{makeStreamTerminal(bufsize)}
}

func NewStreamer(bufsize int) *Streamer {
	s := MakeStreamer(bufsize)
	return &s
}

// Stream data bi-directionally between a connection and a user program's
// stream.  Read buffer size is limited by config.MaxPacketSize.
//
// Buffered state is returned if context is done.  Any I/O error is returned
// (including EOF).  Context error is not returned.
func (s *Streamer) Stream(ctx context.Context, config packet.Service, streamID int32, r io.Reader, w io.Writer, output chan<- packet.Buf,
) (StreamState, error) {
	return stream(ctx, config, streamID, r, w, output, &s.StreamTerminal)
}

// ReStreamer is a streamer restored from suspended state.
type ReStreamer struct {
	StreamTerminal

	sending bool
}

// ReMakeStreamer is for initializing a field.  The value must not be copied.
//
// See ReNewStreamer for other details.
func ReMakeStreamer(state *StreamState, bufsize int) (s ReStreamer, err error) {
	s = ReStreamer{StreamTerminal: makeStreamTerminal(bufsize)}

	var written int
	cover.Cond(len(state.Write.Buffers) == 0, len(state.Write.Buffers) == 1)
	for _, b := range state.Write.Buffers {
		_, err = s.Write(b)
		if err != nil {
			return
		}
		written += len(b)
	}
	cover.MinMax(written, 0, s.Buffer.size()/2)

	if cover.Bool(!state.Write.Receiving) {
		s.WriteEOF()
	}

	s.ReadFlow.Increase(cover.MinMaxInt32(state.Read.Subscribed, 0, math.MaxInt32))
	s.readBuffer = state.Read.Buffer
	cover.Bool(s.readBuffer != nil)
	s.writeSubscribed = cover.MinMaxInt32(state.Write.Subscribed, 0, math.MaxInt32)
	s.sending = cover.Bool(state.Sending)

	*state = StreamState{} // Avoid accidental long-lived buffer references.
	return
}

// ReNewStreamer will consume the state on success.
func ReNewStreamer(state *StreamState, bufsize int) (*ReStreamer, error) {
	s, err := ReMakeStreamer(state, bufsize)
	if err != nil {
		return nil, err
	}

	return &s, err
}

// Stream is like Streamer.Stream.
//
// If the stream is no longer connected, r and w may be nil (causes EOF).
func (s *ReStreamer) Stream(ctx context.Context, config packet.Service, streamID int32, r io.Reader, w io.Writer, output chan<- packet.Buf,
) (StreamState, error) {
	if cover.Bool(!s.sending) {
		output = nil
	}

	return stream(ctx, config, streamID, r, w, output, &s.StreamTerminal)
}

func stream(ctx context.Context, config packet.Service, streamID int32, r io.Reader, w io.Writer, output chan<- packet.Buf, s *StreamTerminal,
) (suspended StreamState, err error) {
	var (
		errRead   error
		errWrite  = errPanicWrite
		writeDone = make(chan struct{})
	)

	go func() {
		defer close(writeDone)
		suspended.Write, errWrite = WriteFrom(ctx, config, streamID, w, output, &s.Buffer, s.writeSubscribed)
	}()

	p := s.readBuffer  // Attempt to drop long-lived reference to initial
	s.readBuffer = nil // packet.  ReadTo will overwrite its argument.
	suspended.Read, errRead = ReadTo(ctx, config, streamID, output, p, &s.ReadFlow, r)

	<-writeDone

	if cover.Bool(!suspended.Write.Receiving) {
		suspended.Read = ReadState{}
	}

	switch {
	case isFailure(errRead):
		err = errRead

	case isFailure(errWrite):
		err = errWrite

	case errRead != nil:
		err = errRead

	case errWrite != nil:
		err = errWrite
	}

	if output == nil {
		return
	}

	suspended.Sending = true

	if cover.Bool(err != nil || !suspended.Write.Receiving) {
		if cover.Bool(len(suspended.Read.Buffer) == 0) { // Avoid sending packets out of order.
			select {
			case output <- packet.Buf(packet.MakeData(config.Code, streamID, 0)):
				suspended.Sending = false

			case <-ctx.Done():
				cover.Location()
			}
		}
	}

	return
}
