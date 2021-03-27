// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"os"

	"gate.computer/gate/internal/container/child"
)

func main() {
	child.ConditionalMain()
	os.Exit(2)
}
