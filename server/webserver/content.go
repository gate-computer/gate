// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webserver

import (
	"errors"
	"io"
)

var errContentTooLong = errors.New("content length limit exceeded")

type contentReader struct {
	r     io.ReadCloser
	space int
}

func newContentReader(r io.ReadCloser, n int) *contentReader {
	return &contentReader{
		r:     r,
		space: n,
	}
}

func (c *contentReader) Read(b []byte) (n int, err error) {
	n, err = c.r.Read(b)
	if n <= c.space {
		c.space -= n
	} else {
		n = c.space
		c.space = 0
		err = errContentTooLong
	}
	return
}

func (c *contentReader) Close() error {
	return c.r.Close()
}
