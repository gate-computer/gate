// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package executable

// Some of these values are also defined in runtime/include/runtime.h

const (
	signalStackReserve = 8192
	StackUsageOffset   = 16 + signalStackReserve + 128
	StackLimitOffset   = StackUsageOffset + 16
)
