// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package packet

import (
	"encoding/binary"
	"math"
)

// Code represents the source or destination of a packet.  It is specific to a
// program instance.
type Code int16

const (
	CodeServices Code = -1
)

type Domain uint8

const (
	DomainCall Domain = iota
	DomainInfo
	DomainFlow
	DomainData
)

func (dom Domain) String() string {
	switch dom {
	case DomainCall:
		return "call"

	case DomainInfo:
		return "info"

	case DomainFlow:
		return "flow"

	case DomainData:
		return "data"
	}

	return dom.invalidString()
}

const (
	Alignment = 8

	// Packet header
	OffsetSize   = 0
	OffsetCode   = 4
	OffsetDomain = 6
	OffsetIndex  = 7
	HeaderSize   = 8

	// Services packet header
	OffsetServicesCount = HeaderSize + 0
	ServicesHeaderSize  = HeaderSize + 2

	// Flow packet header
	FlowHeaderSize = HeaderSize

	// Data packet header
	OffsetDataID   = HeaderSize + 0
	OffsetDataNote = HeaderSize + 4
	DataHeaderSize = HeaderSize + 8
)

const (
	flowOffsetID        = 0
	flowOffsetIncrement = 4
	flowSize            = 8
)

// Align packet length up to a multiple of packet alignment.
func Align(length int) int {
	return (length + (Alignment - 1)) &^ (Alignment - 1)
}

// Buf holds a packet of at least HeaderSize bytes.
type Buf []byte

func Make(code Code, domain Domain, packetSize int) Buf {
	b := Buf(make([]byte, packetSize))
	b.SetCode(code)
	b[OffsetDomain] = byte(domain)
	return b
}

func MakeCall(code Code, contentSize int) Buf {
	return Make(code, DomainCall, HeaderSize+contentSize)
}

func MakeInfo(code Code, contentSize int) Buf {
	return Make(code, DomainInfo, HeaderSize+contentSize)
}

func MakeFlow(code Code, id int32, increment int32) Buf {
	b := MakeFlows(code, 1)
	b.Set(0, id, increment)
	return Buf(b)
}

func MakeFlowEOF(code Code, id int32) Buf {
	return MakeFlow(code, id, 0)
}

func MakeDataEOF(code Code, id int32) Buf {
	return Buf(MakeData(code, id, 0))
}

// MustBeCall panicks if b is not in the call domain.  The value is passed
// through.
func MustBeCall(b Buf) Buf {
	if len(b) < HeaderSize || b.Domain() != DomainCall {
		panic("not a call packet")
	}
	return b
}

// MustBeInfo panicks if b is not in the info domain.  The value is passed
// through.
func MustBeInfo(b Buf) Buf {
	if len(b) < HeaderSize || b.Domain() != DomainInfo {
		panic("not an info packet")
	}
	return b
}

// SetSize encodes the current slice length into the packet header.
func (b Buf) SetSize() {
	if n := len(b); n > math.MaxUint32 {
		panic(n)
	}
	binary.LittleEndian.PutUint32(b[OffsetSize:], uint32(len(b)))
}

// EncodedSize decodes the packet header field.
func (b Buf) EncodedSize() int {
	return int(binary.LittleEndian.Uint32(b[OffsetSize:]))
}

func (b Buf) Code() Code {
	return Code(binary.LittleEndian.Uint16(b[OffsetCode:]))
}

func (b Buf) SetCode(code Code) {
	binary.LittleEndian.PutUint16(b[OffsetCode:], uint16(code))
}

func (b Buf) Domain() Domain {
	return Domain(b[OffsetDomain] & 15)
}

func (b Buf) Index() uint8 {
	return b[OffsetIndex]
}

func (b Buf) SetIndex(i uint8) {
	b[OffsetIndex] = i
}

// Content of a received packet, or buffer for initializing sent packet.
func (b Buf) Content() []byte {
	return b[HeaderSize:]
}

// Split a packet into two parts.  The headerSize parameter determins how many
// bytes are initialized in the second part: the header is copied from the
// first part.  The length of the first part is given as the prefixLen
// parameter.  If the buffer is too short for the second part, the length of
// the second buffer will be zero.
func (b Buf) Split(headerSize, prefixLen int) (prefix, unused Buf) {
	prefixCap := Align(prefixLen)
	if prefixCap > len(b) {
		prefixCap = len(b)
	}

	prefix = b[:prefixLen:prefixCap]
	unused = b[prefixCap:]

	if len(unused) < headerSize {
		unused = unused[0:]
		return
	}

	copy(unused, prefix[:headerSize])
	return
}

// FlowBuf holds a flow packet of at least FlowHeaderSize bytes.
type FlowBuf Buf

func MakeFlows(code Code, count int) FlowBuf {
	return FlowBuf(Make(code, DomainFlow, FlowHeaderSize+count*flowSize))
}

// MustBeFlow panicks if b is not in the flow domain.  The value is passed
// through.
func MustBeFlow(b Buf) FlowBuf {
	if len(b) < FlowHeaderSize || b.Domain() != DomainFlow {
		panic("not a flow packet")
	}
	return FlowBuf(b)
}

func (b FlowBuf) Code() Code {
	return Buf(b).Code()
}

func (b FlowBuf) Num() int {
	return (len(b) - FlowHeaderSize) / flowSize
}

func (b FlowBuf) Get(i int) (id, increment int32) {
	flow := b[FlowHeaderSize+i*flowSize:]
	id = int32(binary.LittleEndian.Uint32(flow[flowOffsetID:]))
	increment = int32(binary.LittleEndian.Uint32(flow[flowOffsetIncrement:]))
	return
}

func (b FlowBuf) Set(i int, id, increment int32) {
	flow := b[FlowHeaderSize+i*flowSize:]
	binary.LittleEndian.PutUint32(flow[flowOffsetID:], uint32(id))
	binary.LittleEndian.PutUint32(flow[flowOffsetIncrement:], uint32(increment))
}

// DataBuf holds a data packet of at least DataHeaderSize bytes.
type DataBuf Buf

func MakeData(code Code, id int32, dataSize int) DataBuf {
	b := Make(code, DomainData, DataHeaderSize+dataSize)
	binary.LittleEndian.PutUint32(b[OffsetDataID:], uint32(id))
	return DataBuf(b)
}

// MustBeData panicks if b is not in the data domain.  The value is passed
// through.
func MustBeData(b Buf) DataBuf {
	if len(b) < DataHeaderSize || b.Domain() != DomainData {
		panic("not a data packet")
	}
	return DataBuf(b)
}

func (b DataBuf) Code() Code {
	return Buf(b).Code()
}

func (b DataBuf) ID() int32 {
	return int32(binary.LittleEndian.Uint32(b[OffsetDataID:]))
}

// Note is a value associated with a data packet.  Each service interface
// specifies its semantics separately.
func (b DataBuf) Note() int32 {
	return int32(binary.LittleEndian.Uint32(b[OffsetDataNote:]))
}

// SetNote value.  It defaults to zero.
func (b DataBuf) SetNote(value int32) {
	binary.LittleEndian.PutUint32(b[OffsetDataNote:], uint32(value))
}

func (b DataBuf) Data() []byte {
	return b[DataHeaderSize:]
}

func (b DataBuf) DataLen() int {
	return len(b) - DataHeaderSize
}

func (b DataBuf) EOF() bool {
	return b.DataLen() == 0
}

func (b DataBuf) Split(dataLen int) (prefix Buf, unused DataBuf) {
	prefix, unusedBuf := Buf(b).Split(DataHeaderSize, DataHeaderSize+dataLen)
	unused = DataBuf(unusedBuf)
	return
}
