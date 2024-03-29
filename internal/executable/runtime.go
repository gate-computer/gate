// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package executable

// See https://pkg.go.dev/gate.computer/wag/compile.
const (
	StackVarsSize    = 64       // Variables at start of stack memory.
	stackSignalSpace = 4832 * 2 // For simultaneous SIGSEGV and SIGXCPU handling.
	StackUsageOffset = StackVarsSize + stackSignalSpace + 240
	StackLimitOffset = StackUsageOffset + 8 + 8
)

const (
	LoaderTextAddr = 0x200000000
)
