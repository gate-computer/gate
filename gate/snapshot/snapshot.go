// Copyright (c) 2020 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package snapshot

import (
	pb "gate.computer/gate/pb/snapshot"
)

const MaxBreakpoints = 100

type Snapshot = pb.Snapshot

func Clone(s *Snapshot) *Snapshot {
	if s == nil {
		return nil
	}
	return &Snapshot{
		Final:         s.Final,
		Trap:          s.Trap,
		Result:        s.Result,
		MonotonicTime: s.MonotonicTime,
		Breakpoints:   append([]uint64(nil), s.Breakpoints...),
	}
}
