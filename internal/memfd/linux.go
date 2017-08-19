// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build linux

package memfd

const (
	_F_LINUX_SPECIFIC_BASE = 1024

	F_ADD_SEALS = FcntlCmd(_F_LINUX_SPECIFIC_BASE + 9)
	F_GET_SEALS = FcntlCmd(_F_LINUX_SPECIFIC_BASE + 10)
)
