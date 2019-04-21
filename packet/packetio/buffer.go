// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package packetio

import (
	"io"
	"math"
	"math/bits"

	"github.com/tsavola/gate/internal/error/badprogram"
	"github.com/tsavola/gate/packet"
	"github.com/tsavola/reach/cover"
)

var errBufferOverflow = badprogram.Errorf("stream data buffer overflow")

// BufferSize is rounded up to a power of two.
func BufferSize(size int) int {
	cover.MinMax(size, 1, (math.MaxInt32+1)/2)
	return 1 << uint(bits.Len32(uint32(size)-1))
}

// PacketBufferSize returns an appropriate data packet size (including header).
func PacketBufferSize(dataSize, maxPacketSize int) int {
	cover.MinMax(maxPacketSize, 65536, math.MaxInt32)

	n := packet.DataHeaderSize + BufferSize(dataSize)
	if cover.Bool(n > maxPacketSize) {
		cover.Cond(maxPacketSize == math.MaxInt32/2)
		n = maxPacketSize
	}

	return n
}

type Buffer struct {
	buf      []byte
	produced Threshold
	consumed Threshold
	eof      bool
}

// MakeBuffer is for initializing a field.  The value must not be copied.
//
// Size will be rounded up to a power of two.
func MakeBuffer(size int) Buffer {
	return Buffer{
		buf:      make([]byte, BufferSize(cover.Min(size, 1))),
		produced: MakeThreshold(),
		consumed: MakeThreshold(),
	}
}

// NewBuffer.
//
// Size will be rounded up to a power of two.
func NewBuffer(size int) *Buffer {
	b := MakeBuffer(size)
	return &b
}

func (b Buffer) size() int {
	return cover.Min(len(b.buf), 1)
}

// Write all data or return an error.
func (b *Buffer) Write(data []byte) (n int, err error) {
	used := b.produced.nonatomic() - b.consumed.Current()
	if uint64(used)+uint64(len(data)) >= uint64(len(b.buf)) {
		err = errBufferOverflow
		return
	}

	mask := uint32(len(b.buf)) - 1
	off := b.produced.nonatomic() & mask
	n = copy(b.buf[off:], data)
	if tail := data[n:]; cover.Bool(len(tail) > 0) {
		n += copy(b.buf, tail)
	}
	b.produced.Increase(int32(n))
	return
}

// WriteEOF before calling Finish.
func (b *Buffer) WriteEOF() {
	b.eof = true
}

// Finish writing.  WriteEOF should be called before this, if applicable.
func (b *Buffer) Finish() {
	b.produced.Finish()
}

// EOF status can be queried before writing has been started or after it has
// been finished.
func (b Buffer) EOF() bool {
	return cover.Bool(b.eof)
}

// endMoved channel will be closed after Finish.
func (b Buffer) endMoved() (c <-chan struct{}) {
	return b.produced.Changed()
}

func (b *Buffer) unwrappedEnd() uint32 {
	return b.produced.Current()
}

func (b Buffer) wrapRange(unwrappedBegin, unwrappedEnd uint32) (off, end int) {
	mask := uint32(len(b.buf)) - 1
	off = int(unwrappedBegin & mask)
	end = int(unwrappedEnd & mask)

	cover.Cond(off < end, off == end, off > end)
	return
}

func (b *Buffer) writeTo(w io.Writer, unwrappedBegin, unwrappedEnd uint32) (n int, err error) {
	var data []byte
	if off, end := b.wrapRange(unwrappedBegin, unwrappedEnd); off <= end {
		if off == end {
			panic("nothing to write")
		}
		data = b.buf[off:end]
	} else {
		data = b.buf[off:] // Just the first half this time.
	}

	n, err = w.Write(data)
	b.consumed.Increase(int32(n))
	return
}

func (b *Buffer) extract(unwrappedBegin, unwrappedEnd uint32) (buffers [][]byte, noEOF bool) {
	switch off, end := b.wrapRange(unwrappedBegin, unwrappedEnd); {
	case off < end:
		buffers = [][]byte{
			b.buf[off:end],
		}

	case off > end:
		buffers = [][]byte{
			b.buf[off:],
			b.buf[:end],
		}
	}

	noEOF = cover.Bool(!b.eof)
	return
}
