// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package packetio

import (
	"io"
	"testing"
)

type testConn struct {
	rbuf int
	wbuf int
	rerr error
	werr error
}

func (c *testConn) SetReadBuffer(n int) (err error) {
	if c.rerr != nil {
		err = c.rerr
		return
	}

	c.rbuf = n
	return
}

func (c *testConn) SetWriteBuffer(n int) (err error) {
	if c.werr != nil {
		err = c.werr
		return
	}

	c.wbuf = n
	return
}

func TestSetBufferSizes(t *testing.T) {
	if err := SetBufferSizes(nil, 32000); err != nil {
		t.Error(err)
	}

	c := &testConn{}
	if err := SetBufferSizes(c, 32000); err != nil {
		t.Error(err)
	}
	if c.rbuf != 32768 {
		t.Error(c.rbuf)
	}
	if c.wbuf != 32768 {
		t.Error(c.wbuf)
	}

	c = &testConn{rerr: io.ErrClosedPipe}
	if err := SetBufferSizes(c, 32000); err != io.ErrClosedPipe {
		t.Error(err)
	}

	c = &testConn{werr: io.ErrClosedPipe}
	if err := SetBufferSizes(c, 32000); err != io.ErrClosedPipe {
		t.Error(err)
	}
}
