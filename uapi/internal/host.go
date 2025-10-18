// Copyright (c) 2025 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !wasm

package internal

import (
	"io"
	"log/slog"
	"net"
	"os"
	"path"
)

// InstanceSocket can be set via linker flag during build process.
// GATE_INSTANCE_SOCKET environment variable will override it.  If neither is
// set, the socket location is guessed.
var InstanceSocket string

func connect() (w io.Writer, r io.Reader) {
	s := os.Getenv("GATE_INSTANCE_SOCKET")
	if s == "" {
		s = InstanceSocket
	}
	if s == "" {
		runDir := os.Getenv("XDG_RUNTIME_DIR") // Gate daemon.
		if runDir == "" {
			runDir = "/run" // Gate server.
		}
		s = path.Join(runDir, "gate", "instance.sock")
	}

	c, err := net.Dial("unix", s)
	if err != nil {
		panic(err)
	}

	slog.Debug("gate: connected", "socket", s)

	return c, c
}
