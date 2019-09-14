// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package packetio

import (
	"testing"
)

func TestBufferSize(t *testing.T) {
	for in, out := range map[int]int{
		1:          1,
		2:          2,
		3:          4,
		4:          4,
		500:        512,
		32768:      32768,
		32769:      65536,
		0x3fffffff: 0x40000000,
		0x40000000: 0x40000000,
	} {
		if r := BufferSize(in); r != out {
			t.Error(in, r)
		}
	}
}

func TestPacketBufferSize(t *testing.T) {
	for _, x := range [][3]int{
		{1, 65536, 17},
		{2, 65536, 18},
		{3, 65536, 20},
		{4, 16, 16},
		{4, 64, 20},
		{4, 65536, 20},
		{4, 131072, 20},
		{500, 65536, 528},
		{32768, 65536, 32784},
		{32769, 65536, 65536},
		{32769, 131072, 65552},
		{0x3fffffff, 65536, 65536},
		{0x3fffffff, 0x7fffffff, 0x40000010},
		{0x40000000, 65536, 65536},
		{0x40000000, 0x3fffffff, 0x3fffffff},
		{0x40000000, 0x7fffffff, 0x40000010},
	} {
		if r := PacketBufferSize(x[0], x[1]); r != x[2] {
			t.Error(x[0], x[1], r)
		}
	}
}

func TestNewBuffer(t *testing.T) {
	if n := NewBuffer(1).size(); n != 1 {
		t.Error(n)
	}

	if n := NewBuffer(123).size(); n != 128 {
		t.Error(n)
	}
}

func TestBufferWriteExtract(t *testing.T) {
	b := NewBuffer(128)

	// Span 1

	data := make([]byte, 120)
	for i := 0; i < len(data); i++ {
		data[i] = byte(i)
	}

	n, err := b.Write(data)
	if err != nil {
		t.Error(err)
	}
	if n != 120 {
		t.Error(n)
	}

	bs, noEOF := b.extract(128*5+50, 128*5+80)
	if !noEOF {
		t.Error("EOF")
	}
	if len(bs[0]) != 30 {
		t.Error(bs)
	}
	if len(bs[1]) != 0 {
		t.Error(bs)
	}
	for i := 50; i < 80; i++ {
		if bs[0][i-50] != byte(i) {
			t.Error(i)
		}
	}

	b.consumed.Increase(120)

	// Span 2

	data = make([]byte, 32)
	for i := 0; i < len(data); i++ {
		data[i] = byte(i)
	}

	n, err = b.Write(data)
	if err != nil {
		t.Error(err)
	}
	if n != 32 {
		t.Error(n)
	}

	b.WriteEOF()

	bs, noEOF = b.extract(120, 120+32)
	if noEOF {
		t.Error("no EOF")
	}
	if len(bs) != 2 {
		t.Error(bs)
	}
	if len(bs[0]) != 8 {
		t.Error(bs)
	}
	if len(bs[1]) != 24 {
		t.Error(bs)
	}
	for i := 0; i < 8; i++ {
		if bs[0][i] != byte(i) {
			t.Error(i)
		}
	}
	for i := 8; i < 32; i++ {
		if bs[1][i-8] != byte(i) {
			t.Error(i)
		}
	}
}
