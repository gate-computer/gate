// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package packetio provides streaming utilities.
//
// Stream, ReadStream and WriteStream are tools for implementing streaming.
// State implements simple binary serialization of suspended I/O state.
package packetio

import (
	"context"
	"io"

	"gate.computer/gate/packet"
)

// Stream is a bidirectional stream between a connection and a channel.
//
// The channel side calls Subscribe, FinishSubscription, Write, CloseWrite, and
// StopTransfer.  The connection side calls Transfer.
//
// Unmarshal may be called before subscription, writing and transfer, and
// Marshal may be called afterwards.
type Stream struct {
	ReadStream
	WriteStream
}

// MakeStream is useful for initializing a field.  Don't use multiple copies of
// the value.
//
// Write buffer size must be a power of two.
func MakeStream(writeBufferSize int) Stream {
	return Stream{
		MakeReadStream(),
		MakeWriteStream(writeBufferSize),
	}
}

// NewStream object.  Use MakeStream when embedding Stream in a struct.
//
// Write buffer size must be a power of two.
func NewStream(writeBufferSize int) *Stream {
	s := MakeStream(writeBufferSize)
	return &s
}

// Unmarshal state from src buffer.  Stream might keep a reference to src until
// Transfer is called.  The unconsumed tail of src is returned.
func (s *Stream) Unmarshal(src []byte, config packet.Service) ([]byte, error) {
	src, err := s.ReadStream.State.Unmarshal(src, config.MaxSendSize)
	if err != nil {
		return src, err
	}

	return s.WriteStream.State.Unmarshal(src, s.WriteStream.bufferSize())
}

// MarshaledSize of the state.
func (s *Stream) MarshaledSize() int {
	return s.ReadStream.State.MarshaledSize() + s.WriteStream.State.MarshaledSize()
}

// Marshal the state into dest buffer.  len(dest) must be at least
// s.MarshaledSize().  The unused tail of dest is returned.
func (s *Stream) Marshal(dest []byte) []byte {
	dest = s.ReadStream.State.Marshal(dest)
	dest = s.WriteStream.State.Marshal(dest)
	return dest
}

// Live state.
//
// The state is undefined during subscription, writing, or transfer.
func (s *Stream) Live() bool {
	return s.ReadStream.Live() || s.WriteStream.Live()
}

func (s *Stream) StopTransfer() {
	s.ReadStream.StopTransfer()
	s.WriteStream.StopTransfer()
}

// Transfer data bidirectionally between a connection (r, w) and a user
// program's stream.  Read buffer size is limited by config.MaxSendSize.
//
// I/O or context errors are returned, excluding EOF.
func (s *Stream) Transfer(ctx context.Context, config packet.Service, streamID int32, r io.Reader, w io.WriteCloser, send chan<- packet.Thunk) error {
	var (
		readDone   = make(chan any, 1)
		readErr    error
		readNormal bool
	)

	go func() {
		defer func() {
			readDone <- recover()
		}()
		readErr = s.ReadStream.Transfer(ctx, config, streamID, send, r)
		readNormal = true
	}()

	writeErr := s.WriteStream.Transfer(ctx, config, streamID, w, send)

	recovered := <-readDone
	if !readNormal {
		panic(recovered)
	}

	if readErr != nil {
		return readErr
	}
	return writeErr
}
