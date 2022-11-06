// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package varint

import (
	"encoding/binary"
	"math"

	"gate.computer/internal/error/badprogram"
)

const MaxLen = binary.MaxVarintLen32

// Scan a non-negative 31-bit integer off the head of a buffer.
func Scan(src []byte) (value int32, tail []byte, err error) {
	tail = src

	x, n := binary.Uvarint(tail)
	if n <= 0 {
		err = badprogram.Error("end of data while decoding varint")
		return
	}
	if x > math.MaxInt32 {
		err = badprogram.Error("varint value out of range")
		return
	}

	value = int32(x)
	tail = tail[n:]
	return
}

// Len of an encoded non-negative 31-bit integer in bytes.
func Len(value int32) int {
	var tmp [MaxLen]byte
	return binary.PutUvarint(tmp[:], uint64(value))
}

// Put a non-negative 31-bit integer at the head of a buffer.
func Put(dest []byte, value int32) (tail []byte) {
	n := binary.PutUvarint(dest, uint64(value))
	return dest[n:]
}
