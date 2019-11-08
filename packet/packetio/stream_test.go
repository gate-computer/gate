// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package packetio

import (
	"testing"

	"github.com/tsavola/gate/packet"
)

var (
	testMaxSendSize int   = 65536
	testService           = packet.Service{MaxSendSize: testMaxSendSize, Code: 1234}
	testStreamID    int32 = 56789
)

type stream interface {
	Live() bool
	Unmarshal([]byte, packet.Service) ([]byte, error)
	MarshaledSize() int
	Marshal([]byte) []byte
}

func marshalUnmarshalStream(t *testing.T, s1, s2 stream) stream {
	b := make([]byte, s1.MarshaledSize())
	if n := len(s1.Marshal(b)); n != 0 {
		t.Error(n)
	}

	if tail, err := s2.Unmarshal(b, testService); err != nil || len(tail) != 0 {
		t.Error(err, len(tail))
	}

	return s2
}
