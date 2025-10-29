// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package packetio

import (
	"math"
	"reflect"
	"testing"

	"gate.computer/gate/packet"
	"github.com/stretchr/testify/assert"

	. "import.name/testing/mustr"
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
	assert.False(t, s1.Live())
	assert.Equal(t, s1.MarshaledSize(), 3)
	b := make([]byte, s1.MarshaledSize())
	assert.Empty(t, s1.Marshal(b))

	s2 := new(State)
	tail := Must(t, R(s2.Unmarshal(b, testMaxSendSize)))
	assert.Equal(t, len(b)-len(tail), 3)
	assert.False(t, s2.Live())

	assert.Equal(t, s1, s2)
}

func testStateUnmarshal(t *testing.T, s1, s2 state, maxSize int) {
	b := make([]byte, s1.MarshaledSize())
	assert.Empty(t, s1.Marshal(b))

	tail := Must(t, R(s2.Unmarshal(b, maxSize)))
	assert.Empty(t, tail)
	assert.True(t, s1.equal(s2))

	_, err := s2.Unmarshal(make([]byte, 2), maxSize)
	assert.Error(t, err)
}

func TestStateUnmarshal(t *testing.T) {
	s1 := InitialState()
	s1.Data = make([]byte, packet.DataHeaderSize)
	s1.Subscribed = math.MaxInt32
	testStateUnmarshal(t, &s1, new(State), testMaxSendSize)
}
