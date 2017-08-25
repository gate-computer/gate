// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package packet

import (
	"encoding/binary"
)

const (
	CodeOffset = 6
	HeaderSize = 8
)

// Code represents the source/destination of a packet.
type Code [2]byte

// Int is mostly useful for debug logging.
func (code Code) Int() int {
	return int(binary.LittleEndian.Uint16(code[:]))
}

// BufSize calculates packet size based on packet content size.
func BufSize(contentSize int) int {
	return HeaderSize + contentSize
}

// Buf holds a packet, including space for its header.
type Buf []byte

// Make a packet buffer.  Code identifies the service for a given program
// instance.  The packet contents may be initialized via the Content method.
func Make(code Code, contentSize int) (b Buf) {
	b = Buf(make([]byte, BufSize(contentSize)))
	copy(b[CodeOffset:], code[:])
	return
}

// Code is the program instance-specific service identifier.
func (b Buf) Code() (code Code) {
	copy(code[:], b[CodeOffset:])
	return
}

// Content of a received packet, or buffer for initializing sent packet.
func (b Buf) Content() []byte {
	return b[HeaderSize:]
}

// ContentSize returns a negative value if b is nil.
func (b Buf) ContentSize() int {
	return len(b) - HeaderSize
}

// Slice returns a subset of the packet buffer.
func (b Buf) Slice(contentSize int) (prefix Buf) {
	prefix = b[:HeaderSize+contentSize]
	return
}

// Split the buffer into two packets.  The first packet's content size is given
// as a parameter.  The first packet must not be extended with append(),
// because the underlying storage is shared with the second packet.  The second
// packet will be nil if there isn't enough space left in the buffer.
func (b Buf) Split(contentSize int) (head, tail Buf) {
	head = b.Slice(contentSize)
	tail = b[HeaderSize+contentSize:]
	if len(tail) >= HeaderSize {
		// make the second packet valid by copying the service code
		copy(tail[CodeOffset:], head[CodeOffset:CodeOffset+2])
	} else {
		tail = nil
	}
	return
}
