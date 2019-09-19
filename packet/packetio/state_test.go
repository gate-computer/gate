// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package packetio

import (
	"encoding/binary"
	"math"
	"reflect"
	"testing"

	"github.com/tsavola/gate/internal/varint"
	"github.com/tsavola/gate/packet"
)

var put16 = binary.LittleEndian.PutUint16

func canonicalReadState(s ReadState) ReadState {
	s.Buffer = append([]byte{}, s.Buffer...)
	return s
}

func readStateEqual(s1, s2 ReadState) bool {
	return reflect.DeepEqual(canonicalReadState(s1), canonicalReadState(s2))
}

func TestReadStateZero(t *testing.T) {
	var s1 ReadState
	if s1.isMeaningful() {
		t.Error("zero read state is meaningful")
	}
	if s1.Size() == 0 {
		t.Error("zero read state has no size")
	}
	b := make([]byte, s1.Size())
	if n := len(s1.Marshal(b)); n != 0 {
		t.Error(n)
	}

	var s2 ReadState
	tail, err := s2.Unmarshal(b, testService)
	if err != nil {
		t.Error(err)
	}
	if n := len(b) - len(tail); n != 3 {
		t.Error(s1.Size(), n)
	}
	if s2.isMeaningful() {
		t.Error("unmarshaled zero read state is meaningful")
	}

	if !reflect.DeepEqual(s1, s2) {
		t.Error(s1, s2)
	}
}

func TestReadStateUnmarshal(t *testing.T) {
	p := packet.MakeData(testService.Code, testStreamID, testService.MaxPacketSize-packet.DataHeaderSize)
	s1 := ReadState{
		Buffer:     p,
		Subscribed: math.MaxInt32,
	}
	b := make([]byte, s1.Size())
	if n := len(s1.Marshal(b)); n != 0 {
		t.Error(n)
	}

	var s2 ReadState
	if tail, err := s2.Unmarshal(b, testService); err != nil || len(tail) != 0 {
		t.Error(err, len(tail))
	}
	if !reflect.DeepEqual(s1, s2) {
		t.Error(s1, s2)
	}

	if _, err := new(ReadState).Unmarshal(nil, testService); err == nil {
		t.Error("no buffer")
	}
	if _, err := new(ReadState).Unmarshal(b[:10], testService); err == nil {
		t.Error("partial state")
	}

	headerLen := 1 + varint.Len(math.MaxInt32) + varint.Len(int32(len(p)))

	bad := append([]byte{}, b...)
	put16(bad[headerLen+packet.OffsetCode:], uint16(testService.Code)-1)
	if _, err := new(ReadState).Unmarshal(bad, testService); err == nil {
		t.Error("wrong code")
	}

	bad = append([]byte{}, b...)
	bad[headerLen+packet.OffsetDomain] = byte(packet.DomainFlow)
	if _, err := new(ReadState).Unmarshal(bad, testService); err == nil {
		t.Error("wrong domain")
	}

	bad = append([]byte{}, b...)
	bad[headerLen+7] = 0xff
	if _, err := new(ReadState).Unmarshal(bad, testService); err == nil {
		t.Error("reserved byte with incorrect value")
	}
}

func canonicalWriteState(s WriteState) WriteState {
	s.Buffers[0] = append(s.Buffers[0], s.Buffers[1]...)
	s.Buffers[1] = nil
	return s
}

func writeStateEqual(s1, s2 WriteState) bool {
	return reflect.DeepEqual(canonicalWriteState(s1), canonicalWriteState(s2))
}

func TestWriteStateZero(t *testing.T) {
	var s1 WriteState
	if s1.isMeaningful() {
		t.Error("zero write state is meaningful")
	}
	if s1.Size() == 0 {
		t.Error("zero write state has no size")
	}
	b := make([]byte, s1.Size())
	if n := len(s1.Marshal(b)); n != 0 {
		t.Error(n)
	}

	var s2 WriteState
	b, err := s2.Unmarshal(b, 512)
	if err != nil {
		t.Error(err)
	}
	if len(b) != 0 {
		t.Error(s1.Size(), len(b))
	}
	if s2.isMeaningful() {
		t.Error("unmarshaled zero write state is meaningful")
	}

	if !writeStateEqual(s1, s2) {
		t.Error(s1, s2)
	}
}

func TestWriteStateUnmarshal(t *testing.T) {
	t.Run("SingleBuffer", func(t *testing.T) {
		testWriteStateUnmarshal(t, [2][]byte{
			make([]byte, 512),
			nil,
		})
	})

	t.Run("SplitBuffer", func(t *testing.T) {
		testWriteStateUnmarshal(t, [2][]byte{
			make([]byte, 303),
			make([]byte, 209),
		})
	})
}

func testWriteStateUnmarshal(t *testing.T, buffers [2][]byte) {
	s1 := WriteState{
		Buffers:    buffers,
		Subscribed: math.MaxInt32,
		Receiving:  true,
	}
	b := make([]byte, s1.Size())
	if n := len(s1.Marshal(b)); n != 0 {
		t.Error(n)
	}

	var s2 WriteState
	if tail, err := s2.Unmarshal(b, 512); err != nil || len(tail) != 0 {
		t.Error(err, len(tail))
	}
	if !writeStateEqual(s1, s2) {
		t.Error(s1, s2)
	}

	if _, err := new(WriteState).Unmarshal(nil, 512); err == nil {
		t.Error("no buffer")
	}
	if _, err := new(WriteState).Unmarshal(b[:10], 512); err == nil {
		t.Error("partial state")
	}
}

func canonicalStreamState(s StreamState) StreamState {
	s.Read = canonicalReadState(s.Read)
	s.Write = canonicalWriteState(s.Write)
	return s
}

func streamStateEqual(s1, s2 StreamState) bool {
	return reflect.DeepEqual(canonicalStreamState(s1), canonicalStreamState(s2))
}

func TestStreamStateZero(t *testing.T) {
	var s1 StreamState
	if s1.IsMeaningful() {
		t.Error("zero stream state is meaningful")
	}
	if s1.Size() == 0 {
		t.Error("zero stream state has no size")
	}
	b := make([]byte, s1.Size())
	if n := len(s1.Marshal(b)); n != 0 {
		t.Error(n)
	}

	var s2 StreamState
	b, err := s2.Unmarshal(b, testService, 512)
	if err != nil {
		t.Error(err)
	}
	if len(b) != 0 {
		t.Error(s1.Size(), len(b))
	}
	if s2.IsMeaningful() {
		t.Error("unmarshaled zero stream state is meaningful")
	}

	if !streamStateEqual(s1, s2) {
		t.Error(s1, s2)
	}
}

func TestStreamStateBusy(t *testing.T) {
	p := packet.MakeData(testService.Code, testStreamID, testService.MaxPacketSize-packet.DataHeaderSize)
	s1 := StreamState{
		Write: WriteState{
			Buffers: [2][]byte{
				make([]byte, 511),
				nil,
			},
			Subscribed: math.MaxInt32,
			Receiving:  true,
		},
		Read: ReadState{
			Buffer:     p,
			Subscribed: math.MaxInt32,
		},
		Sending: true,
	}
	if !s1.IsMeaningful() {
		t.Error("busy stream state is not meaningful")
	}
	b := make([]byte, s1.Size())
	if n := len(s1.Marshal(b)); n != 0 {
		t.Error(n)
	}

	var s2 StreamState
	b, err := s2.Unmarshal(b, testService, 512)
	if err != nil {
		t.Error(err)
	}
	if len(b) != 0 {
		t.Error(s1.Size(), len(b))
	}
	if !s2.IsMeaningful() {
		t.Error("unmarshaled busy stream state is not meaningful")
	}

	if !streamStateEqual(s1, s2) {
		t.Error(s1, s2)
	}
}

func TestStreamStateUnmarshalError(t *testing.T) {
	p := packet.MakeData(testService.Code, testStreamID, testService.MaxPacketSize-packet.DataHeaderSize)
	s := StreamState{
		Write: WriteState{
			Buffers: [2][]byte{
				make([]byte, 511),
				nil,
			},
			Subscribed: math.MaxInt32,
			Receiving:  true,
		},
		Read: ReadState{
			Buffer:     p,
			Subscribed: math.MaxInt32,
		},
		Sending: true,
	}
	b := make([]byte, s.Size())
	if n := len(s.Marshal(b)); n != 0 {
		t.Error(n)
	}

	if _, err := new(StreamState).Unmarshal(nil, testService, 512); err == nil {
		t.Error("no buffer")
	}
	if _, err := new(StreamState).Unmarshal(b[:15], testService, 512); err == nil {
		t.Error("partial read state")
	}
	if _, err := new(StreamState).Unmarshal(b[:1+s.Read.Size()], testService, 512); err == nil {
		t.Error("no write state")
	}
	if _, err := new(StreamState).Unmarshal(b[:1+s.Read.Size()+10], testService, 512); err == nil {
		t.Error("partial write state")
	}
}

func TestStreamStateMarshalToLongerBuffer(t *testing.T) {
	var s StreamState
	if n := len(s.Marshal(make([]byte, s.Size()+123))); n != 123 {
		t.Error(n)
	}
}
