// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package packetio

// RWBufferer does internal buffering.
type RWBufferer interface {
	SetReadBuffer(bytes int) error
	SetWriteBuffer(bytes int) error
}

// SetBufferSizes set the connection's internal read and write buffer sizes,
// unless it is nil.
func SetBufferSizes(conn RWBufferer, size int) error {
	if conn != nil {
		size = BufferSize(size)

		if err := conn.SetReadBuffer(size); err != nil {
			return err
		}

		if err := conn.SetWriteBuffer(size); err != nil {
			return err
		}
	}

	return nil
}
