// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package snapshot

type Flags uint32

const (
	FlagTerminated Flags = 1 << 0
)

// Terminated indicates that the program should not be resumed - it should only
// be inspected for debugging purposes.
func (f Flags) Terminated() bool { return f&FlagTerminated != 0 }
func (f *Flags) SetTerminated()  { *f |= FlagTerminated }

// Service state representation.
type Service struct {
	Name   string
	Buffer []byte
}

// Buffers of a suspended, halted or terminated program.  Contents are
// undefined while the program is running.
//
// Services, Input, and Output array contents are not mutated, but the arrays
// may be replaced.  Buffers can be reused by making shallow copies.
type Buffers struct {
	Services []Service
	Input    []byte // Buffered data which the program hasn't received yet.
	Output   []byte // Buffered data which the program has already sent.
	Flags
}
