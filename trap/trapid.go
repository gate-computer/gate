// Copyright (c) 2020 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package trap enumerates trap identifiers.
package trap

import (
	wagtrap "github.com/tsavola/wag/trap"
)

type ID wagtrap.ID

const (
	Exit                          = ID(wagtrap.Exit)
	NoFunction                    = ID(wagtrap.NoFunction)
	Suspended                     = ID(wagtrap.Suspended)
	Unreachable                   = ID(wagtrap.Unreachable)
	CallStackExhausted            = ID(wagtrap.CallStackExhausted)
	MemoryAccessOutOfBounds       = ID(wagtrap.MemoryAccessOutOfBounds)
	IndirectCallIndexOutOfBounds  = ID(wagtrap.IndirectCallIndexOutOfBounds)
	IndirectCallSignatureMismatch = ID(wagtrap.IndirectCallSignatureMismatch)
	IntegerDivideByZero           = ID(wagtrap.IntegerDivideByZero)
	IntegerOverflow               = ID(wagtrap.IntegerOverflow)
	Breakpoint                    = ID(wagtrap.Breakpoint)
	ABIDeficiency                 = ID(27)
	ABIViolation                  = ID(28)
	InternalError                 = ID(29)
	Killed                        = ID(30)
)

func (id ID) String() string {
	switch id {
	case ABIDeficiency:
		return "ABI deficiency"

	case ABIViolation:
		return "ABI violation"

	case InternalError:
		return "internal error"

	case Killed:
		return "killed"

	default:
		return wagtrap.ID(id).String()
	}
}
