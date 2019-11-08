// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package packetio

import (
	"github.com/tsavola/gate/internal/error/badprogram"
	"github.com/tsavola/gate/internal/varint"
	"github.com/tsavola/gate/packet"
)

const (
	errStateTooShort   = badprogram.Err("unexpected end of serialized state")
	errBufferStateSize = badprogram.Err("serialized buffer state size limit exceeded")
)

const minReadStateSize = minCommonStateHeaderSize + 0

// ReadState of a suspended ReadTo call.
//
// Zero value may not represent the initial state.
type ReadState struct {
	Buffer     packet.DataBuf // Data to be sent to program.
	Subscribed int32          // Additional data that could be read.
}

// MakeReadState is for initializing a field.  The value must not be copied.
//
// See NewReadState.
func MakeReadState() ReadState {
	return ReadState{} // For now the zero value represents the initial state.
}

// NewReadState creates a representation of initial read state.
func NewReadState() *ReadState {
	s := MakeReadState()
	return &s
}

func (s *ReadState) isMeaningful() bool {
	return len(s.Buffer) > 0
}

// Size of marshaled state.
func (s *ReadState) Size() (n int) {
	n += commonStateSize(s.Subscribed, len(s.Buffer))
	n += len(s.Buffer)
	return
}

// Marshal the state into a buffer.  len(b) must be at least s.Size().
func (s *ReadState) Marshal(dest []byte) (tail []byte) {
	tail = marshalCommonStateHeader(dest, 0, s.Subscribed, len(s.Buffer))

	copy(tail, s.Buffer)
	clearPacketSizeField(tail)
	tail = tail[len(s.Buffer):]

	return
}

// Unmarshal state from a buffer.  ReadState might keep a reference to the
// buffer.
func (s *ReadState) Unmarshal(src []byte, config packet.Service) (tail []byte, err error) {
	var size int

	*s = ReadState{}

	_, s.Subscribed, size, tail, err = unmarshalCommonStateHeader(src, config.MaxSendSize)
	if err != nil {
		return
	}

	if size != 0 {
		s.Buffer, err = packet.ImportData(tail[:size:size], config.Code)
		if err != nil {
			return
		}
		tail = tail[size:]
	}

	return
}

const minWriteStateSize = minCommonStateHeaderSize + 0
const writeStateFlagReceiving uint8 = 1 << 0

// WriteState of a suspended WriteFrom call.
//
// Zero value may not represent the initial state.
type WriteState struct {
	Buffers    [2][]byte // Data to be written to connection.
	Subscribed int32     // Additional data that the program could sent.
	Receiving  bool      // Program hasn't sent EOF data packet?
}

// MakeWriteState is for initializing a field.  The value must not be copied.
//
// See NewWriteState.
func MakeWriteState() WriteState {
	return WriteState{
		Receiving: true,
	}
}

// NewWriteState creates a representation of initial write state.
func NewWriteState() *WriteState {
	s := MakeWriteState()
	return &s
}

func (s *WriteState) bufferSize() (n int) {
	for _, b := range s.Buffers {
		n += len(b)
	}
	return
}

func (s *WriteState) isMeaningful() bool {
	return s.bufferSize() > 0 || s.Receiving
}

// Size of marshaled state.
func (s *WriteState) Size() (n int) {
	n += commonStateSize(s.Subscribed, s.bufferSize())

	for _, b := range s.Buffers {
		n += len(b)
	}

	return
}

// Marshal the state into a buffer.  len(b) must be at least s.Size().
func (s *WriteState) Marshal(dest []byte) (tail []byte) {
	var flags uint8
	if s.Receiving {
		flags |= writeStateFlagReceiving
	}

	tail = marshalCommonStateHeader(dest, flags, s.Subscribed, s.bufferSize())

	for _, b := range s.Buffers {
		copy(tail, b)
		tail = tail[len(b):]
	}

	return
}

// Unmarshal state from a buffer.  WriteState might keep a reference to the
// buffer.
func (s *WriteState) Unmarshal(src []byte, writeBufSize int) (tail []byte, err error) {
	var flags uint8
	var size int

	*s = WriteState{}

	flags, s.Subscribed, size, tail, err = unmarshalCommonStateHeader(src, writeBufSize)
	if err != nil {
		return
	}

	s.Receiving = flags&writeStateFlagReceiving != 0

	if size != 0 {
		s.Buffers[0] = tail[:size:size]
		tail = tail[size:]
	}

	return
}

// Discard buffered data.
func (s *WriteState) Discard() {
	for i := range s.Buffers {
		s.Buffers[i] = nil
	}
}

const minStreamStateSize = 1 + minReadStateSize + minWriteStateSize
const streamStateFlagSending uint8 = 1 << 0

// StreamState of a suspended Stream call.
//
// Zero value may not represent the initial state.
type StreamState struct {
	Write   WriteState
	Read    ReadState
	Sending bool // EOF data packet hasn't been sent to program?
}

// MakeStreamState is for initializing a field.  The value must not be copied.
//
// See NewStreamState.
func MakeStreamState() StreamState {
	return StreamState{
		Write:   MakeWriteState(),
		Read:    MakeReadState(),
		Sending: true,
	}
}

// NewStreamState creates a representation of initial stream state.
func NewStreamState() *StreamState {
	s := MakeStreamState()
	return &s
}

// IsMeaningful returns false if restoring a stream with this state would be
// practically equivalent to having no stream at all.
func (s *StreamState) IsMeaningful() bool {
	return s.Write.isMeaningful() || s.Read.isMeaningful() || s.Sending
}

// Size of marshaled state.
func (s *StreamState) Size() (n int) {
	n += 1 // Flags.
	n += s.Read.Size()
	n += s.Write.Size()
	return
}

// Marshal the state into a buffer.  len(b) must be at least s.Size().
func (s *StreamState) Marshal(dest []byte) (tail []byte) {
	var flags uint8
	if s.Sending {
		flags |= streamStateFlagSending
	}
	dest[0] = flags
	tail = dest[1:]

	tail = s.Read.Marshal(tail)

	tail = s.Write.Marshal(tail)

	return
}

// Unmarshal state from a buffer.  StreamState might keep a reference to the
// buffer.
func (s *StreamState) Unmarshal(src []byte, config packet.Service, writeBufSize int,
) (tail []byte, err error) {
	*s = StreamState{}

	if len(src) < minStreamStateSize {
		err = errStateTooShort
		return
	}

	flags := src[0]
	tail = src[1:]

	s.Sending = flags&streamStateFlagSending != 0

	tail, err = s.Read.Unmarshal(tail, config)
	if err != nil {
		return
	}

	tail, err = s.Write.Unmarshal(tail, writeBufSize)
	if err != nil {
		return
	}

	return
}

const minCommonStateHeaderSize = 1 + 1 + 1

func commonStateSize(subscribed int32, size int) (n int) {
	n += 1 // Flags.
	n += varint.Len(subscribed)
	n += varint.Len(int32(size))
	return
}

func marshalCommonStateHeader(dest []byte, flags uint8, subscribed int32, size int) (tail []byte) {
	dest[0] = flags
	tail = dest[1:]

	tail = varint.Put(tail, subscribed)

	tail = varint.Put(tail, int32(size))

	return
}

func unmarshalCommonStateHeader(src []byte, maxSize int,
) (flags uint8, subscribed int32, size int, tail []byte, err error) {
	if len(src) < minCommonStateHeaderSize {
		err = errStateTooShort
		return
	}

	flags = src[0]
	tail = src[1:]

	subscribed, tail, err = varint.Scan(tail)
	if err != nil {
		return
	}

	s, tail, err := varint.Scan(tail)
	if err != nil {
		return
	}
	size = int(s)
	if size > maxSize {
		err = errBufferStateSize
		return
	}
	if len(tail) < size {
		err = errStateTooShort
		return
	}

	return
}

func clearPacketSizeField(b packet.DataBuf) {
	for i := packet.OffsetSize; i < packet.OffsetSize+4 && i < len(b); i++ {
		b[i] = 0
	}
}
