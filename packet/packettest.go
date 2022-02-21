// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package packet

import (
	"gate.computer/gate/internal/error/grpc"
)

type err string

func (s err) Error() string       { return string(s) }
func (s err) PublicError() string { return string(s) }
func (s err) BadRequest() bool    { return true }
func (s err) BadProgram() bool    { return true }
func (s err) Status() int         { return 400 } // Bad Request
func (s err) GRPCCode() int       { return grpc.InvalidArgument }

const errInvalidCall = err("invalid call packet")
const errInvalidData = err("invalid data packet")

// IsValidCall checks service call packet's or call result packet's header.
// Packet content is disregarded.
func IsValidCall(b []byte, c Code) bool {
	return isValidHeader(b, HeaderSize, c, DomainCall)
}

// IsValidInfo checks service info packet's header.  Packet content is
// disregarded.
func IsValidInfo(b []byte, c Code) bool {
	return isValidHeader(b, HeaderSize, c, DomainInfo)
}

// IsValidFlow checks stream flow packet, including the flow entries.
func IsValidFlow(b []byte, c Code) bool {
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
func IsValidData(b []byte, c Code) bool {
	if !isValidHeader(b, DataHeaderSize, c, DomainData) {
		return false
	}

	return DataBuf(b).ID() >= 0
}

// ImportCall packet, validating it leniently.  The buffer is NOT copied.
func ImportCall(b []byte, c Code) (p Buf, err error) {
	if !isValidHeader(b, HeaderSize, c, DomainCall) {
		err = errInvalidCall
		return
	}

	p = Buf(b)
	return
}

// ImportData packet, validating it leniently.  The buffer is NOT copied.
func ImportData(b []byte, c Code) (p DataBuf, err error) {
	if !isValidHeader(b, DataHeaderSize, c, DomainData) || DataBuf(b).ID() < 0 {
		err = errInvalidData
		return
	}

	p = DataBuf(b)
	return
}

func isValidHeader(b []byte, n int, c Code, d Domain) bool {
	return len(b) >= n && Buf(b).Code() == c && Buf(b).Domain() == d
}
