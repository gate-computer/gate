// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package packetio provides streaming utilities.
//
// Stream is a high-level tool for implementing bi-directional streaming.  It
// is built on the ReadTo, WriteFrom, Buffer and Threshold primitives.
//
// StreamState, ReadState and WriteState implement simple binary serialization
// of suspended I/O state.
package packetio

import (
	"context"
	"errors"
	"io"

	"github.com/tsavola/gate/packet"
)

var errPanicWrite = errors.New("panic while writing")

func isFailure(err error) bool {
	return err != nil && err != io.EOF
}

// Stream (bi-directional).
//
// User program side calls the Subscribe, Write, WriteEOF, and Finish methods.
// The other side calls the Stream method.
type Stream struct {
	writeBuf        Buffer
	writeSubscribed int32
	readFlow        Threshold
	readBuf         packet.DataBuf
	sending         bool
}

// MakeStream is for initializing a field.  The value must not be
// copied.
func MakeStream(bufsize int) Stream {
	return Stream{
		writeBuf: MakeBuffer(bufsize),
		readFlow: MakeThreshold(),
		sending:  true,
	}
}

func NewStream(bufsize int) *Stream {
	s := MakeStream(bufsize)
	return &s
}

// Write all data or return an error.
func (s *Stream) Write(data []byte) (n int, err error) {
	return s.writeBuf.Write(data)
}

// Subscribe to more data (i.e. read).
func (s *Stream) Subscribe(increment int32) error {
	return s.readFlow.Increase(increment)
}

// WriteEOF before calling Finish.
func (s *Stream) WriteEOF() {
	s.writeBuf.WriteEOF()
}

// Finish reading and writing.  WriteEOF should be called before this, if
// applicable.
func (s *Stream) Finish() {
	s.writeBuf.Finish()
	s.readFlow.Finish()
}

// EOF status can be queried before writing has been started or after it has
// been finished.
func (s Stream) EOF() bool {
	return s.writeBuf.EOF()
}

// Restore state.
//
// Stream won't keep references to StreamState's buffers, but copies data as
// needed.
func (s *Stream) Restore(state StreamState) (err error) {
	for _, b := range state.Write.Buffers {
		if len(b) > 0 {
			_, err = s.Write(b)
			if err != nil {
				return
			}
		}
	}

	if !state.Write.Receiving {
		s.WriteEOF()
	}

	s.writeSubscribed = state.Write.Subscribed

	s.Subscribe(state.Read.Subscribed)

	if len(state.Read.Buffer) > 0 {
		s.readBuf = append(packet.DataBuf{}, state.Read.Buffer...)
		s.readBuf.Sanitize()
	}

	s.sending = state.Sending
	return
}

// Transfer data bi-directionally between a connection (r, w) and a user
// program's stream.  Read buffer size is limited by config.MaxPacketSize.
//
// If the connection has been disconnected, r and w may be nil (causes EOF).
//
// Any I/O error is returned (including EOF).  Context error is not returned
// (function will return normally when context is done).
func (s *Stream) Transfer(ctx context.Context, config packet.Service, streamID int32, r io.Reader, w io.Writer, output chan<- packet.Buf,
) (state StreamState, err error) {
	if !s.sending {
		output = nil
	}

	var (
		errRead   error
		errWrite  = errPanicWrite
		writeDone = make(chan struct{})
	)

	go func() {
		defer close(writeDone)
		state.Write, errWrite = WriteFrom(ctx, config, streamID, w, output, &s.writeBuf, s.writeSubscribed)
	}()

	p := s.readBuf  // Attempt to drop long-lived reference to initial
	s.readBuf = nil // packet.  ReadTo will overwrite its argument.
	state.Read, errRead = ReadTo(ctx, config, streamID, output, p, &s.readFlow, r)

	<-writeDone

	if !state.Write.Receiving {
		state.Read = ReadState{}
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

	state.Sending = true

	if err != nil || !state.Write.Receiving {
		if len(state.Read.Buffer) == 0 { // Avoid sending packets out of order.
			select {
			case output <- packet.Buf(packet.MakeData(config.Code, streamID, 0)):
				state.Sending = false

			case <-ctx.Done():
				return
			}
		}
	}

	return
}
