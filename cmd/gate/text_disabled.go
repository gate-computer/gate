// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !gatedumptext

package main

import (
	"errors"

	"gate.computer/wag/section"
)

func dumpText(text []byte, funcAddrs []uint32, ns *section.NameSection) error {
	return errors.New("cmd/gate must be compiled with build tag: gatedumptext")
}
