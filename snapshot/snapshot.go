// Copyright (c) 2020 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package snapshot

import (
	"github.com/tsavola/gate/trap"
)

type Flags uint64

const (
	FlagFinal Flags = 1 << iota
	FlagDebugInfo
)

// Final indicates that the instance should not be resumed - it should only be
// inspected for debugging purposes.
func (f Flags) Final() bool { return f&FlagFinal != 0 }

// DebugInfo indicates that extra debugging features should be enabled, even if
// they cause significant time and space overhead.
func (f Flags) DebugInfo() bool { return f&FlagDebugInfo != 0 }

type Snapshot struct {
	Flags
	Trap          trap.ID
	Result        int32 // Meaningful when Trap is Exit.
	MonotonicTime uint64
	Breakpoints   []uint64
}
