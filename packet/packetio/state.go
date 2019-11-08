// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package packetio

import (
	"errors"

	"github.com/tsavola/gate/internal/error/badprogram"
	"github.com/tsavola/gate/internal/varint"
)

const (
	errStateTooShort   = badprogram.Err("unexpected end of serialized state")
	errBufferStateSize = badprogram.Err("serialized buffer state size limit exceeded")
)

// State flags.
const (
	FlagSubscribing = 1 << iota
	FlagReadWriting
	FlagSendReceiving
)

// State of a suspended unidirectional stream.
//
// Zero value represents a final state, not the initial state.
type State struct {
	Flags      uint32
	Subscribed uint32
	Data       []byte
}

func InitialState() State {
	return InitialStateWithDataBuffer(nil)
}

func InitialStateWithDataBuffer(databuf []byte) State {
	return State{
		Flags: FlagSubscribing | FlagReadWriting | FlagSendReceiving,
		Data:  databuf[:0],
	}
}

// Live?
func (s *State) Live() bool {
	return s.Flags&(FlagSubscribing|FlagReadWriting|FlagSendReceiving) != 0
}

// Unmarshal state from src buffer.  State might keep a reference to src,
// unless a preallocated buffer was passed to InitialStateWithDataBuffer.  The
// unconsumed tail of src is returned.
//
// maxDataSize is the Data buffer size limit.  For ReadStream state, it should
// be config.MaxSendSize where config is the Transfer argument.  For
// WriteStream state, it should match the stream's bufsize.
func (s *State) Unmarshal(src []byte, maxDataSize int) ([]byte, error) {
	var err error

	*s = State{}

	flags, src, err := varint.Scan(src)
	if err != nil {
		return src, err
	}
	s.Flags = uint32(flags)

	subscribed, src, err := varint.Scan(src)
	if err != nil {
		return src, err
	}
	s.Subscribed = uint32(subscribed)

	size, src, err := varint.Scan(src)
	if err != nil {
		return src, err
	}
	if int(size) > maxDataSize {
		return src, errBufferStateSize
	}
	if len(src) < int(size) {
		return src, errStateTooShort
	}
	if size != 0 {
		data := src[:size:size]
		src = src[size:]

		if s.Data == nil {
			s.Data = data
		} else {
			if cap(s.Data) >= len(data) {
				s.Data = s.Data[:len(data)]
			} else {
				s.Data = make([]byte, len(data))
			}
			copy(s.Data, data)
		}
	}

	if s.Flags&FlagSendReceiving == 0 && (s.Flags&FlagReadWriting != 0 || size != 0) {
		return src, errors.New("impossible state")
	}

	return src, nil
}

// MarshaledSize of the state.
func (s *State) MarshaledSize() (n int) {
	n += varint.Len(int32(s.Flags))
	n += varint.Len(int32(s.Subscribed))
	n += varint.Len(int32(len(s.Data)))
	n += len(s.Data)
	return
}

// Marshal the state into dest buffer.  len(dest) must be at least
// s.MarshaledSize().  The unused tail of dest is returned.
func (s *State) Marshal(dest []byte) []byte {
	dest = varint.Put(dest, int32(s.Flags))
	dest = varint.Put(dest, int32(s.Subscribed))
	dest = varint.Put(dest, int32(len(s.Data)))
	copy(dest, s.Data)
	return dest[len(s.Data):]
}
