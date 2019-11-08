// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package packetio

import (
	"math"
	"reflect"
	"testing"

	"github.com/tsavola/gate/packet"
)

type state interface {
	Live() bool
	Unmarshal([]byte, int) ([]byte, error)
	MarshaledSize() int
	Marshal([]byte) []byte
	canonical() state
	equal(state) bool
}

func (s *State) canonical() state {
	clone := *s
	if len(clone.Data) == 0 {
		clone.Data = nil
	}
	return &clone
}

func (s *State) equal(s2 state) bool {
	return reflect.DeepEqual(s.canonical(), s2.canonical())
}

func TestStateZero(t *testing.T) {
	s1 := new(State)
	if s1.Live() {
		t.Error("zero value is live")
	}
	if n := s1.MarshaledSize(); n != 3 {
		t.Error(n)
	}
	b := make([]byte, s1.MarshaledSize())
	if n := len(s1.Marshal(b)); n != 0 {
		t.Error(n)
	}

	s2 := new(State)
	tail, err := s2.Unmarshal(b, testMaxSendSize)
	if err != nil {
		t.Error(err)
	}
	if n := len(b) - len(tail); n != 3 {
		t.Error(s1.MarshaledSize(), n)
	}
	if s2.Live() {
		t.Error("unmarshaled zero state is live")
	}

	if !reflect.DeepEqual(s1, s2) {
		t.Error(s1, s2)
	}
}

func testStateUnmarshal(t *testing.T, s1, s2 state, maxSize int) {
	b := make([]byte, s1.MarshaledSize())
	if n := len(s1.Marshal(b)); n != 0 {
		t.Error(n)
	}

	if tail, err := s2.Unmarshal(b, maxSize); err != nil || len(tail) != 0 {
		t.Error(err, len(tail))
	}
	if !s1.equal(s2) {
		t.Error(s1, s2)
	}

	if _, err := s2.Unmarshal(make([]byte, 2), maxSize); err == nil {
		t.Error("unmarshaling from short buffer succeeded")
	}
}

func TestStateUnmarshal(t *testing.T) {
	s1 := InitialState()
	s1.Data = make([]byte, packet.DataHeaderSize)
	s1.Subscribed = math.MaxInt32
	testStateUnmarshal(t, &s1, new(State), testMaxSendSize)
}
