// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"gate.computer/cmd/gate-daemon/daemon"
	_ "modernc.org/sqlite"
)

func main() {
	daemon.Main()
}
