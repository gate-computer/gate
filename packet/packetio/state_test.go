// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package packetio

import (
	"encoding/binary"
	"math"
	"reflect"
	"testing"

	"github.com/tsavola/gate/packet"
)

func TestReadStateZero(t *testing.T) {
	s1 := ReadState{}
	if s1.IsMeaningful() {
		t.Error("zero read state is meaningful")
	}
	b := make([]byte, s1.Size())
	s1.Marshal(b)

	var s2 ReadState
	n, err := s2.Unmarshal(b, testService)
	if err != nil {
		t.Error(err)
	}
	if n != s1.Size() {
		t.Error(n)
	}
	if s2.IsMeaningful() {
		t.Error("unmarshaled zero read state is meaningful")
	}

	if !reflect.DeepEqual(s1, s2) {
		t.Error(s1, s2)
	}
}

func TestReadStateUnmarshalError(t *testing.T) {
	p := packet.MakeData(testService.Code, testStreamID, testService.MaxPacketSize-packet.DataHeaderSize)
	s1 := ReadState{
		Buffer:     p,
		Subscribed: math.MaxInt32,
	}
	b := make([]byte, s1.Size())
	s1.Marshal(b)

	if _, err := new(ReadState).Unmarshal(nil, testService); err == nil {
		t.Error("no buffer")
	}
	if _, err := new(ReadState).Unmarshal(b[:10], testService); err == nil {
		t.Error("partial state")
	}

	bad := append([]byte{}, b...)
	binary.LittleEndian.PutUint32(bad[1+4+packet.OffsetSize:], 0)
	if _, err := new(ReadState).Unmarshal(bad, testService); err == nil {
		t.Error("zero packet size")
	}

	bad = append([]byte{}, b...)
	binary.LittleEndian.PutUint32(bad[1+4+packet.OffsetSize:], 0x7fffffff)
	if _, err := new(ReadState).Unmarshal(bad, testService); err == nil {
		t.Error("packet size out of bounds")
	}

	bad = append([]byte{}, b...)
	binary.LittleEndian.PutUint16(bad[1+4+packet.OffsetCode:], uint16(testService.Code)-1)
	if _, err := new(ReadState).Unmarshal(bad, testService); err == nil {
		t.Error("wrong code")
	}

	bad = append([]byte{}, b...)
	bad[1+4+packet.OffsetDomain] = byte(packet.DomainFlow)
	if _, err := new(ReadState).Unmarshal(bad, testService); err == nil {
		t.Error("wrong domain")
	}

	bad = append([]byte{}, b...)
	bad[1+4+7] = 0xff // Reserved byte must be passed through.
	if _, err := new(ReadState).Unmarshal(bad, testService); err != nil {
		t.Error(err)
	}
}

func TestWriteStateZero(t *testing.T) {
	s1 := WriteState{}
	if s1.IsMeaningful() {
		t.Error("zero write state is meaningful")
	}
	b := make([]byte, s1.Size())
	s1.Marshal(b)

	var s2 WriteState
	n, err := s2.Unmarshal(b, 512)
	if err != nil {
		t.Error(err)
	}
	if n != s1.Size() {
		t.Error(n)
	}
	if s2.IsMeaningful() {
		t.Error("unmarshaled zero write state is meaningful")
	}

	if !reflect.DeepEqual(s1, s2) {
		t.Error(s1, s2)
	}
}

func TestWriteStateUnmarshalError(t *testing.T) {
	s1 := WriteState{
		Buffers:    [][]byte{make([]byte, 511)},
		Subscribed: math.MaxInt32,
		Receiving:  true,
	}
	b := make([]byte, s1.Size())
	s1.Marshal(b)

	if _, err := new(WriteState).Unmarshal(nil, 512); err == nil {
		t.Error("no buffer")
	}
	if _, err := new(WriteState).Unmarshal(b[:10], 512); err == nil {
		t.Error("partial state")
	}
}

func TestStreamStateZero(t *testing.T) {
	s1 := StreamState{}
	if s1.IsMeaningful() {
		t.Error("zero stream state is meaningful")
	}
	b := make([]byte, s1.Size())
	s1.Marshal(b)

	var s2 StreamState
	n, err := s2.Unmarshal(b, testService, 512)
	if err != nil {
		t.Error(err)
	}
	if n != s1.Size() {
		t.Error(n)
	}
	if s2.IsMeaningful() {
		t.Error("unmarshaled zero stream state is meaningful")
	}

	if !reflect.DeepEqual(s1, s2) {
		t.Error(s1, s2)
	}
}

func TestStreamStateBusy(t *testing.T) {
	p := packet.MakeData(testService.Code, testStreamID, testService.MaxPacketSize-packet.DataHeaderSize)
	s1 := StreamState{
		Write: WriteState{
			Buffers:    [][]byte{make([]byte, 511)},
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
	s1.Marshal(b)

	var s2 StreamState
	n, err := s2.Unmarshal(b, testService, 512)
	if err != nil {
		t.Error(err)
	}
	if n != s1.Size() {
		t.Error(n)
	}
	if !s2.IsMeaningful() {
		t.Error("unmarshaled busy stream state is not meaningful")
	}

	binary.LittleEndian.PutUint32(p[packet.OffsetSize:], uint32(len(p))) // For comparison.
	if !reflect.DeepEqual(s1, s2) {
		t.Error(s1, s2)
	}
}

func TestStreamStateUnmarshalError(t *testing.T) {
	p := packet.MakeData(testService.Code, testStreamID, testService.MaxPacketSize-packet.DataHeaderSize)
	s1 := StreamState{
		Write: WriteState{
			Buffers:    [][]byte{make([]byte, 511)},
			Subscribed: math.MaxInt32,
			Receiving:  true,
		},
		Read: ReadState{
			Buffer:     p,
			Subscribed: math.MaxInt32,
		},
		Sending: true,
	}
	b := make([]byte, s1.Size())
	s1.Marshal(b)

	if _, err := new(StreamState).Unmarshal(nil, testService, 512); err == nil {
		t.Error("no buffer")
	}
	if _, err := new(StreamState).Unmarshal(b[:15], testService, 512); err == nil {
		t.Error("partial read state")
	}
	if _, err := new(StreamState).Unmarshal(b[:1+4+s1.Read.Size()], testService, 512); err == nil {
		t.Error("no write state size")
	}
	if _, err := new(StreamState).Unmarshal(b[:1+4+s1.Read.Size()+10], testService, 512); err == nil {
		t.Error("partial write state")
	}

	bad := append([]byte{}, b...)
	binary.LittleEndian.PutUint32(bad[1+4+1+4+packet.OffsetSize:], packet.HeaderSize)
	if _, err := new(StreamState).Unmarshal(bad, testService, 512); err == nil {
		t.Error("read packet size out of bounds")
	}

	bad = append([]byte{}, b...)
	binary.LittleEndian.PutUint32(bad[1+4+s1.Read.Size()+4+1+4:], 513)
	if _, err := new(StreamState).Unmarshal(bad, testService, 512); err == nil {
		t.Error("write buffer size out of bounds")
	}
}
