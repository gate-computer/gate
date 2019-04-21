// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package packetio

import (
	"encoding/binary"
	"errors"
	"math"

	"github.com/tsavola/gate/packet"
	"github.com/tsavola/reach/cover"
)

// ReadState of a suspended ReadTo call.
type ReadState struct {
	Buffer     packet.DataBuf // Data to be sent to program.
	Subscribed int32          // Additional data that could be read.
}

// IsMeaningful returns false if resuming the state would cause no observable
// effects.
func (s ReadState) IsMeaningful() bool {
	return cover.Bool(len(s.Buffer) != 0)
}

// Size of marshaled state.
func (s ReadState) Size() (n int) {
	packetLen := len(s.Buffer)
	if packetLen <= packet.DataHeaderSize {
		cover.Cond(packetLen == 0, packetLen == packet.DataHeaderSize)
		packetLen = 4
	} else {
		cover.Min(packetLen, packet.DataHeaderSize+1)
	}

	n += 1 // Flags.
	n += 4 // Subscribed.
	n += packetLen
	return
}

// Marshal the state into a buffer.  len(b) must be at least s.Size().
func (s ReadState) Marshal(b []byte) {
	b[0] = 0 // No flags.
	b = b[1:]

	binary.LittleEndian.PutUint32(b, uint32(cover.MinMaxInt32(s.Subscribed, 0, math.MaxInt32)))
	b = b[4:]

	packetLen := len(s.Buffer)
	if packetLen <= packet.DataHeaderSize {
		cover.Cond(packetLen == 0, packetLen == packet.DataHeaderSize)
		packetLen = 4
	} else {
		cover.Min(packetLen, packet.DataHeaderSize+1)
		copy(b, s.Buffer)
	}
	binary.LittleEndian.PutUint32(b[packet.OffsetSize:], uint32(packetLen))
	b = b[packetLen:]

	cover.Min(len(b), 0)
}

// Unmarshal state from a buffer.  ReadState might keep a reference to the
// buffer.
func (s *ReadState) Unmarshal(b []byte, config packet.Service) (n int, err error) {
	if len(b) < 1+4+4 {
		err = errors.New("stream read state too short")
		return
	}

	n += 1 // Flags.

	s.Subscribed = cover.MinMaxInt32(int32(binary.LittleEndian.Uint32(b[n:])), 0, math.MaxInt32)
	if s.Subscribed < 0 {
		err = errors.New("stream read subscription is negative")
		return
	}
	n += 4

	size := binary.LittleEndian.Uint32(b[n+packet.OffsetSize:])
	if size < 4 {
		err = errors.New("packet buffer size is inconsistent")
		return
	}
	if size > uint32(config.MaxPacketSize) {
		err = errors.New("packet buffer size exceeds maximum packet size")
		return
	}
	if len(b) < n+int(size) {
		err = errors.New("stream read state is inconsistent")
		return
	}

	if size == 4 {
		s.Buffer = nil
	} else {
		cover.MinMaxUint32(size, packet.DataHeaderSize+1, uint32(config.MaxPacketSize))

		if size < packet.DataHeaderSize {
			err = errors.New("data packet is too small")
			return
		}

		p := packet.Buf(b[n : n+int(size) : n+int(size)])
		if p.Code() != config.Code {
			err = errors.New("data packet has incorrect service code")
			return
		}
		if p.Domain() != packet.DomainData {
			err = errors.New("data packet has incorrect domain")
			return
		}

		s.Buffer = packet.DataBuf(p)
	}
	n += int(size)

	cover.Min(len(b), n)
	return
}

// WriteState of a suspended WriteFrom call.
type WriteState struct {
	Buffers    [][]byte // Data to be written to connection.
	Subscribed int32    // Additional data that the program could sent.
	Receiving  bool     // Program hasn't sent EOF data packet?
}

// IsMeaningful returns false if resuming the state would cause no observable
// effects.
func (s WriteState) IsMeaningful() bool {
	return cover.Bool(len(s.Buffers) != 0 || s.Receiving)
}

// Size of marshaled state.
func (s WriteState) Size() (n int) {
	n += 1 // Flags.
	n += 4 // Subscribed.
	n += 4 // Concatenated buffer size (excluding this size field).

	cover.MinMax(len(s.Buffers), 0, 2)
	for _, b := range s.Buffers {
		n += len(b)
	}

	cover.MinMax(n, 1+4+4+0, 1+4+4+math.MaxInt32)
	return
}

// Marshal the state into a buffer.  len(b) must be at least s.Size().
func (s WriteState) Marshal(b []byte) {
	var flags uint8
	if cover.Bool(s.Receiving) {
		flags = 1
	}
	b[0] = flags
	b = b[1:]

	binary.LittleEndian.PutUint32(b, uint32(cover.MinMaxInt32(s.Subscribed, 0, math.MaxInt32)))
	b = b[4:]

	var size uint32
	for _, buf := range s.Buffers {
		size += uint32(len(buf))
	}
	binary.LittleEndian.PutUint32(b, cover.MinUint32(size, 0))
	b = b[4:]

	cover.MinMax(len(s.Buffers), 0, 2)
	for _, buf := range s.Buffers {
		b = b[copy(b, buf):]
	}

	cover.Min(len(b), 0)
}

// Unmarshal state from a buffer.  WriteState might keep a reference to the
// buffer.
func (s *WriteState) Unmarshal(b []byte, writeBufSize int) (n int, err error) {
	if len(b) < 1+4+4 {
		err = errors.New("stream write state too short")
		return
	}

	s.Receiving = cover.Bool(b[n]&1 != 0)
	n += 1

	s.Subscribed = cover.MinMaxInt32(int32(binary.LittleEndian.Uint32(b[n:])), 0, math.MaxInt32)
	if s.Subscribed < 0 {
		err = errors.New("stream write subscription is negative")
		return
	}
	n += 4

	size := cover.MinMaxUint32(binary.LittleEndian.Uint32(b[n:]), 0, uint32(writeBufSize))
	n += 4

	if size > uint32(writeBufSize) {
		err = errors.New("stream write buffer is too large")
		return
	}
	if len(b) < n+int(size) {
		err = errors.New("stream write state is inconsistent")
		return
	}

	if size == 0 {
		s.Buffers = nil
	} else {
		s.Buffers = [][]byte{
			b[n : n+int(size) : n+int(size)],
		}
		n += int(size)
	}

	cover.Min(len(b), n)
	return
}

// StreamState of a suspended Stream call.
type StreamState struct {
	Write   WriteState
	Read    ReadState
	Sending bool // EOF data packet hasn't been sent to program?
}

// IsMeaningful returns false if resuming the state would cause no observable
// effects.
func (s StreamState) IsMeaningful() bool {
	return cover.Bool(s.Write.IsMeaningful() || s.Read.IsMeaningful() || s.Sending)
}

// Size of marshaled state.
func (s StreamState) Size() (n int) {
	n += 1 // Flags.
	n += 4 // ReadState size.
	if cover.Bool(s.Read.IsMeaningful()) {
		n += s.Read.Size()
	}
	n += 4 // WriteState size.
	if cover.Bool(s.Write.IsMeaningful()) {
		n += s.Write.Size()
	}
	return
}

// Marshal the state into a buffer.  len(b) must be at least s.Size().
func (s StreamState) Marshal(b []byte) {
	var flags uint8
	if cover.Bool(s.Sending) {
		flags = 1
	}
	b[0] = flags
	b = b[1:]

	if s.Read.IsMeaningful() {
		binary.LittleEndian.PutUint32(b, uint32(s.Read.Size()))
		b = b[4:]

		s.Read.Marshal(b)
		b = b[s.Read.Size():]
	} else {
		binary.LittleEndian.PutUint32(b, 0)
		b = b[4:]
	}

	if s.Write.IsMeaningful() {
		binary.LittleEndian.PutUint32(b, uint32(s.Write.Size()))
		b = b[4:]

		s.Write.Marshal(b)
		b = b[s.Write.Size():]
	} else {
		binary.LittleEndian.PutUint32(b, 0)
		b = b[4:]
	}
}

// Unmarshal state from a buffer.  StreamState might keep a reference to the
// buffer.
func (s *StreamState) Unmarshal(b []byte, config packet.Service, writeBufSize int) (n int, err error) {
	if len(b) < 1+4+4 {
		err = errors.New("stream state too short")
		return
	}

	s.Sending = cover.Bool(b[n]&1 != 0)
	n += 1

	size := binary.LittleEndian.Uint32(b[n:])
	n += 4

	if size == 0 {
		s.Read = ReadState{}
	} else {
		if uint64(len(b)) < uint64(n)+uint64(size) {
			err = errors.New("stream state is inconsistent")
			return
		}

		_, err = s.Read.Unmarshal(b[n:n+int(size)], config)
		if err != nil {
			return
		}
		n += int(size)
	}

	if len(b) < n+4 {
		err = errors.New("stream state is too short")
		return
	}

	size = binary.LittleEndian.Uint32(b[n:])
	n += 4

	if size == 0 {
		s.Write = WriteState{}
	} else {
		if uint64(len(b)) < uint64(n)+uint64(size) {
			err = errors.New("stream state is inconsistent")
			return
		}

		_, err = s.Write.Unmarshal(b[n:n+int(size)], writeBufSize)
		if err != nil {
			return
		}
		n += int(size)
	}

	return
}
