// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wasm

// Custom WebAssembly sections.
const (
	ServiceSection = "gate.service" // May appear once after code section.
	IOSection      = "gate.io"      // May appear once after service section.
	BufferSection  = "gate.buffer"  // May appear once after io section.
	StackSection   = "gate.stack"   // May appear once between buffer and data sections.
)
