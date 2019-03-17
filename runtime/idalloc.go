// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"math/bits"
)

const idAllocRangeLen = 16384

func makeIdAllocator(limit int) (<-chan int16, chan<- int16) {
	if true {
		c := makeIdAllocatorImpl1(limit)
		return c, c
	} else {
		return makeIdAllocatorImpl2(limit)
	}
}

func makeIdAllocatorImpl1(limit int) chan int16 {
	c := make(chan int16, limit)
	for i := 0; i < limit; i++ {
		c <- int16(i)
	}
	return c
}

func makeIdAllocatorImpl2(limit int) (<-chan int16, chan<- int16) {
	alloc := make(chan int16, 128)
	free := make(chan int16, 128)
	go serveIdAllocations(alloc, free, limit)
	return alloc, free
}

func serveIdAllocations(alloc chan<- int16, free <-chan int16, limit int) {
	defer close(alloc)

	var ar idAllocator
	ar.init(limit)

	allocatedId := int16(-1)

	for {
		if allocatedId < 0 {
			allocatedId = ar.alloc()
			if allocatedId < 0 {
				id, ok := <-free
				if !ok {
					return
				}
				ar.free(id)
			}
		}

		select {
		case alloc <- allocatedId:
			allocatedId = -1

		case id, ok := <-free:
			if !ok {
				return
			}
			ar.free(id)
		}
	}
}

// idAllocator allocates ids in range [2,32767] in arbitrary order.
type idAllocator struct {
	level3 [512]uint64
	level2 [8]uint64
	level1 uint8
	minId  int16
}

func (ar *idAllocator) init(limit int) {
	for i := 0; i < len(ar.level3); i++ {
		ar.level3[i] = ^uint64(0)
	}
	for i := 0; i < len(ar.level2); i++ {
		ar.level2[i] = ^uint64(0)
	}
	ar.level1 = ^uint8(0)
	ar.minId = int16(16384 - limit)
}

func (ar *idAllocator) alloc() int16 {
	bit1 := uint(bits.Len8(ar.level1)) - 1
	bit2 := uint(bits.Len64(ar.level2[bit1])) - 1
	bit3 := uint(bits.Len64(ar.level3[bit1<<6|bit2])) - 1
	id := int16(bit1<<12 | bit2<<6 | bit3)
	if id < ar.minId {
		return -1
	}

	ar.level3[bit1<<6|bit2] ^= 1 << bit3
	if ar.level3[bit1<<6|bit2] == 0 {
		ar.level2[bit1] ^= 1 << bit2
		if ar.level2[bit1] == 0 {
			ar.level1 ^= 1 << bit1
		}
	}

	return id
}

func (ar *idAllocator) free(id int16) {
	bit1 := uint(id >> 12)
	bit2 := uint(id>>6) & 63
	bit3 := uint(id) & 63

	ar.level3[bit1<<6|bit2] |= 1 << bit3
	ar.level2[bit1] |= 1 << bit2
	ar.level1 |= 1 << bit1
}
