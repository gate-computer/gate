// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package snapshot

// Service state representation.
type Service struct {
	Name   string
	Buffer []byte
}

// Buffers of a suspended program.  Contents are empty if the program is in
// observable state but not suspended.  Contents are undefined while the
// program is running.
type Buffers struct {
	Services []Service
	Input    []byte // Buffered data which the program hasn't received yet.
	Output   []byte // Buffered data which the program has already sent.
}
