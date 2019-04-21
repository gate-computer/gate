// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package packetio

import (
	"testing"

	"github.com/tsavola/gate/packet"
	"github.com/tsavola/reach"
)

var (
	testService        = packet.Service{MaxPacketSize: 65536, Code: 1234}
	testStreamID int32 = 56789
)

func TestMain(m *testing.M) {
	reach.TestMain(m)
}
