// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build !dumptext

package main

import (
	"github.com/tsavola/wag/section"
)

func prepareTextDump(text []byte) []byte {
	return nil
}

func dumpText(text []byte, funcAddrs []uint32, ns *section.NameSection) error {
	return nil
}
