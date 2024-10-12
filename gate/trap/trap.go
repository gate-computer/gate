// Copyright (c) 2020 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package trap enumerates trap identifiers.
package trap

import (
	pb "gate.computer/gate/pb/trap"
)

type ID = pb.ID

const (
	Exit                          = pb.ID_EXIT
	NoFunction                    = pb.ID_NO_FUNCTION
	Suspended                     = pb.ID_SUSPENDED
	Unreachable                   = pb.ID_UNREACHABLE
	CallStackExhausted            = pb.ID_CALL_STACK_EXHAUSTED
	MemoryAccessOutOfBounds       = pb.ID_MEMORY_ACCESS_OUT_OF_BOUNDS
	IndirectCallIndexOutOfBounds  = pb.ID_INDIRECT_CALL_INDEX_OUT_OF_BOUNDS
	IndirectCallSignatureMismatch = pb.ID_INDIRECT_CALL_SIGNATURE_MISMATCH
	IntegerDivideByZero           = pb.ID_INTEGER_DIVIDE_BY_ZERO
	IntegerOverflow               = pb.ID_INTEGER_OVERFLOW
	Breakpoint                    = pb.ID_BREAKPOINT
	ABIDeficiency                 = pb.ID_ABI_DEFICIENCY
	ABIViolation                  = pb.ID_ABI_VIOLATION
	InternalError                 = pb.ID_INTERNAL_ERROR
	Killed                        = pb.ID_KILLED
)
