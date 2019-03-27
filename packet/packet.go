// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package packet

import (
	"encoding/binary"
	"fmt"
	"strconv"
)

// Code represents the source or destination of a packet.
type Code int16

const (
	CodeServices Code = -1
)

func (code Code) String() string {
	switch {
	case code >= 0:
		return fmt.Sprintf("service[%d]", code)

	case code == CodeServices:
		return "services"

	default:
		return fmt.Sprintf("<invalid code %d>", code)
	}
}

type Domain uint8

const (
	DomainCall Domain = iota
	DomainState
	DomainFlow
	DomainData
)

func (d Domain) String() string {
	switch d {
	case DomainCall:
		return "call"

	case DomainState:
		return "state"

	case DomainFlow:
		return "flow"

	case DomainData:
		return "data"

	default:
		return "<invalid domain>"
	}
}

const (
	// Packet header
	OffsetSize     = 0
	OffsetCode     = 4
	OffsetDomain   = 6
	offsetReserved = 7
	HeaderSize     = 8

	// Services packet header
	OffsetServicesCount = HeaderSize + 0
	ServicesHeaderSize  = HeaderSize + 8

	// Flow packet header
	FlowHeaderSize = HeaderSize

	// Data packet header
	OffsetDataID       = HeaderSize + 0
	offsetDataReserved = HeaderSize + 4
	DataHeaderSize     = HeaderSize + 8
)

const (
	flowOffsetID        = 0
	flowOffsetIncrement = 4
	flowSize            = 8
)

// Buf holds a packet.
type Buf []byte

func Make(code Code, domain Domain, packetSize int) Buf {
	b := Buf(make([]byte, packetSize))
	binary.LittleEndian.PutUint16(b[OffsetCode:], uint16(code))
	b[OffsetDomain] = byte(domain)
	return b
}

func MakeFlow(code Code, id int32, increment uint32) Buf {
	b := MakeFlows(code, 1)
	b.Set(0, id, increment)
	return Buf(b)
}

// Code is the program instance-specific service identifier.
func (b Buf) Code() Code {
	return Code(binary.LittleEndian.Uint16(b[OffsetCode:]))
}

func (b Buf) Domain() Domain {
	return Domain(b[OffsetDomain])
}

// Content of a received packet, or buffer for initializing sent packet.
func (b Buf) Content() []byte {
	return b[HeaderSize:]
}

// Slice off the tail of a packet.
func (b Buf) Slice(packetSize int) (prefix Buf) {
	return b[:packetSize]
}

func (b Buf) String() (s string) {
	var (
		size     string
		reserved string
	)

	if n := binary.LittleEndian.Uint32(b); n == 0 || n == uint32(len(b)) {
		size = strconv.Itoa(len(b))
	} else {
		size = fmt.Sprintf("%d/%d", n, len(b))
	}

	if x := b[offsetReserved]; x != 0 {
		reserved = fmt.Sprintf(" reserved=0x%02x", x)
	}

	s = fmt.Sprintf("size=%s code=%s domain=%s%s", size, b.Code(), b.Domain(), reserved)

	switch b.Domain() {
	case DomainFlow:
		s += FlowBuf(b).string()

	case DomainData:
		s += DataBuf(b).string()
	}
	return
}

// Split a packet into two parts.  The headerSize parameter determins how many
// bytes are initialized in the second part: the header is copied from the
// first part.  The length (and capacity) of the first part is given as the
// packetSize parameter.  If the buffer is too short for the second part, nil
// is returned.
func (b Buf) Split(headerSize, packetSize int) (prefix, unused Buf) {
	prefix = b[:packetSize:packetSize]
	unused = b[len(prefix):]

	if len(unused) < headerSize {
		unused = nil
		return
	}

	copy(unused, prefix[:headerSize])
	return
}

// FlowBuf holds a flow packet.
type FlowBuf Buf

func MakeFlows(code Code, count int) FlowBuf {
	b := Make(code, DomainFlow, FlowHeaderSize+count*flowSize)
	return FlowBuf(b)
}

func (b FlowBuf) Num() int {
	return (len(b) - FlowHeaderSize) / flowSize
}

func (b FlowBuf) Get(i int) (id int32, increment uint32) {
	flow := b[FlowHeaderSize+i*flowSize:]
	id = int32(binary.LittleEndian.Uint32(flow[flowOffsetID:]))
	increment = binary.LittleEndian.Uint32(flow[flowOffsetIncrement:])
	return
}

func (b FlowBuf) Set(i int, id int32, increment uint32) {
	flow := b[FlowHeaderSize+i*flowSize:]
	binary.LittleEndian.PutUint32(flow[flowOffsetID:], uint32(id))
	binary.LittleEndian.PutUint32(flow[flowOffsetIncrement:], increment)
}

func (b FlowBuf) String() string {
	return Buf(b).String() + b.string()
}

func (b FlowBuf) string() (s string) {
	for i := 0; i < b.Num(); i++ {
		id, inc := b.Get(i)
		s += fmt.Sprintf(" stream[%d]+=%d", id, inc)
	}
	return
}

// DataBuf holds a data packet.
type DataBuf Buf

func MakeData(code Code, id int32, dataSize int) DataBuf {
	b := Make(code, DomainData, DataHeaderSize+dataSize)
	binary.LittleEndian.PutUint32(b[OffsetDataID:], uint32(id))
	return DataBuf(b)
}

func (b DataBuf) ID() int32 {
	return int32(binary.LittleEndian.Uint32(b[OffsetDataID:]))
}

func (b DataBuf) Data() []byte {
	return b[DataHeaderSize:]
}

func (b DataBuf) DataLen() int {
	return len(b) - DataHeaderSize
}

func (b DataBuf) Split(dataLen int) (prefix Buf, unused DataBuf) {
	prefix, unusedBuf := Buf(b).Split(DataHeaderSize, DataHeaderSize+dataLen)
	unused = DataBuf(unusedBuf)
	return
}

func (b DataBuf) String() string {
	return Buf(b).String() + b.string()
}

func (b DataBuf) string() string {
	if b.DataLen() == 0 {
		return fmt.Sprintf(" id=%d eof", b.ID())
	}
	return fmt.Sprintf(" id=%d datalen=%d", b.ID(), b.DataLen())
}
