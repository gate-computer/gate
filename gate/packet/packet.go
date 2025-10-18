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
	flowOffsetID    = 0
	flowOffsetValue = 4
	flowSize        = 8
)

// Thunk may be called once to acquire a packet.  It returns an empty buffer if
// no packet was available after all.
type Thunk func() (Buf, error)

// Align packet length up to a multiple of packet alignment.
func Align(length int) int {
	return (length + (Alignment - 1)) &^ (Alignment - 1)
}

// Buf holds a packet of at least HeaderSize bytes.
type Buf []byte

func Make(code Code, domain Domain, packetSize int) Buf {
	b := Buf(make([]byte, packetSize, Align(packetSize)))
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

func MakeFlow(code Code, id, value int32) Buf {
	b := MakeFlows(code, 1)
	b.SetFlow(0, id, value)
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

// Cut a packet into two parts.  The headerSize parameter determins how
// many bytes are initialized in the second part: the header is copied
// from the first part.  The length of the first part is given as the
// prefixLen parameter.  If the buffer is too short for the second part,
// the length of the second buffer will be zero.
func (b Buf) Cut(headerSize, prefixLen int) (prefix, unused Buf) {
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

// Thunk returns a function which returns the packet.
func (b Buf) Thunk() Thunk {
	return func() (Buf, error) { return b, nil }
}

// Flow change of a stream.
type Flow struct {
	ID    int32 // Stream ID.
	Value int32 // Positive increment, EOF indicator (0), or negative note.
}

func (f Flow) IsIncrement() bool { return f.Value > 0 }
func (f Flow) IsEOF() bool       { return f.Value == 0 }
func (f Flow) IsNote() bool      { return f.Value < 0 }

// Increment is positive (if ok).
func (f Flow) Increment() (n int32, ok bool) {
	if f.Value > 0 {
		return f.Value, true
	}
	return 0, false
}

// Note is negative (if ok).
func (f Flow) Note() (n int32, ok bool) {
	if f.Value < 0 {
		return f.Value, true
	}
	return 0, false
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

func (b FlowBuf) Len() int {
	return (len(b) - FlowHeaderSize) / flowSize
}

func (b FlowBuf) At(i int) Flow {
	flow := b[FlowHeaderSize+i*flowSize:]
	return Flow{
		int32(binary.LittleEndian.Uint32(flow[flowOffsetID:])),
		int32(binary.LittleEndian.Uint32(flow[flowOffsetValue:])),
	}
}

func (b FlowBuf) Set(i int, f Flow) {
	b.SetFlow(i, f.ID, f.Value)
}

func (b FlowBuf) SetFlow(i int, id, value int32) {
	flow := b[FlowHeaderSize+i*flowSize:]
	binary.LittleEndian.PutUint32(flow[flowOffsetID:], uint32(id))
	binary.LittleEndian.PutUint32(flow[flowOffsetValue:], uint32(value))
}

// Thunk returns a function which returns the packet.
func (b FlowBuf) Thunk() Thunk {
	return Buf(b).Thunk()
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

func (b DataBuf) Cut(dataLen int) (prefix Buf, unused DataBuf) {
	prefix, unusedBuf := Buf(b).Cut(DataHeaderSize, DataHeaderSize+dataLen)
	unused = DataBuf(unusedBuf)
	return
}

// Thunk returns a function which returns the packet.
func (b DataBuf) Thunk() Thunk {
	return Buf(b).Thunk()
}
