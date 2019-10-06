// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build wasm

package packet

func (code Code) String() string {
	switch {
	case code >= 0:
		return "service"

	case code == CodeServices:
		return "services"

	default:
		return "invalid"
	}
}

func (b Buf) String() string {
	return "packet"
}

func (b FlowBuf) String() string {
	return "flow packet"
}

func (b DataBuf) String() string {
	return "data packet"
}
