// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package packetio

import (
	"gate.computer/gate/packet"
)

var (
	testMaxSendSize int   = 65536
	testService           = packet.Service{MaxSendSize: testMaxSendSize, Code: 1234}
	testStreamID    int32 = 56789
)
