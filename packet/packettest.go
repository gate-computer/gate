// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package packet

// IsValidCall checks service call packet's or call result packet's header.
// Packet content is disregarded.
func IsValidCall(b Buf, c Code) bool {
	return isValidHeader(b, HeaderSize, c, DomainCall)
}

// IsValidState checks service state packet's header.  Packet content is
// disregarded.
func IsValidState(b Buf, c Code) bool {
	return isValidHeader(b, HeaderSize, c, DomainState)
}

// IsValidFlow checks stream flow packet, including the flow entries.
func IsValidFlow(b Buf, c Code) bool {
	if !isValidHeader(b, FlowHeaderSize, c, DomainFlow) {
		return false
	}

	if len(b)&7 != 0 {
		return false
	}

	p := FlowBuf(b)
	for i := 0; i < p.Num(); i++ {
		if id, increment := p.Get(i); id < 0 || increment < 0 {
			return false
		}
	}
	return true
}

// IsValidData checks stream data packet's header.  Data is disregarded.
func IsValidData(b Buf, c Code) bool {
	if !isValidHeader(b, DataHeaderSize, c, DomainData) {
		return false
	}

	return DataBuf(b).ID() >= 0 && isZeros(b[offsetDataReserved:DataHeaderSize])
}

func isValidHeader(b Buf, n int, c Code, d Domain) bool {
	return len(b) >= n && b.Code() == c && b.Domain() == d && b[offsetReserved] == 0
}

func isZeros(b []byte) bool {
	for _, x := range b {
		if x != 0 {
			return false
		}
	}
	return true
}
