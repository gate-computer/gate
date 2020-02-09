// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build gatedumptext

package main

import (
	"os"

	objdump "github.com/tsavola/wag/object/debug/dump"
	"github.com/tsavola/wag/section"
)

func dumpText(text []byte, funcAddrs []uint32, ns *section.NameSection) error {
	return objdump.Text(os.Stdout, text, 0, funcAddrs, ns)
}
