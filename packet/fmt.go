// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build !wasm

package packet

import (
	"encoding/binary"
	"fmt"
	"strconv"
)

func (dom Domain) invalidString() string {
	return fmt.Sprintf("<invalid domain %d>", dom)
}

func (code Code) String() string {
	switch {
	case code >= 0:
		return fmt.Sprintf("service[%d]", code)

	case code == CodeServices:
		return "services"

	default:
		return fmt.Sprintf("<invalid code %d>", code)
	}
}

func (b Buf) String() (s string) {
	var (
		size  string
		index string
	)

	if n := binary.LittleEndian.Uint32(b); n == 0 || n == uint32(len(b)) {
		size = strconv.Itoa(len(b))
	} else {
		size = fmt.Sprintf("%d/%d", n, len(b))
	}

	if i := b.Index(); i != 0 {
		index = fmt.Sprintf(" index=%d", i)
	}

	s = fmt.Sprintf("size=%s code=%s domain=%s%s", size, b.Code(), b.Domain(), index)

	switch b.Domain() {
	case DomainFlow:
		s += FlowBuf(b).string()

	case DomainData:
		s += DataBuf(b).string()
	}
	return
}

func (b FlowBuf) String() string {
	return Buf(b).String() + b.string()
}

func (b FlowBuf) string() (s string) {
	for i := 0; i < b.Num(); i++ {
		id, inc := b.Get(i)
		s += fmt.Sprintf(" stream[%d]+=%d", id, inc)
	}
	return
}

func (b DataBuf) String() string {
	return Buf(b).String() + b.string()
}

func (b DataBuf) string() (s string) {
	s = fmt.Sprintf(" id=%d", b.ID())
	if n := b.DataLen(); n > 0 {
		s += fmt.Sprintf(" datalen=%d", n)
	}
	if x := b.Note(); x != 0 {
		s += fmt.Sprintf(" note=%d", x)
	}
	return
}
