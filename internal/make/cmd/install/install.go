// Copyright (c) 2022 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"io/ioutil"

	"gate.computer/gate/internal/container/common"
	. "import.name/make"
)

func main() { Main(targets, "") }

// These are set via -ldflags by main.go.
var (
	PREFIX     string
	LIBEXECDIR string
)

func targets() (targets Tasks) {
	var (
		DESTDIR  = Getvar("DESTDIR", "")
		BINDIR   = Getvar("BINDIR", Join(PREFIX, "bin"))
		SHAREDIR = Getvar("SHAREDIR", Join(PREFIX, "share"))
	)

	lib := targets.Add(TargetDefault("lib",
		Installation(DESTDIR+LIBEXECDIR+"/", Join("lib/gate", common.ExecutorFilename), true),
		Installation(DESTDIR+LIBEXECDIR+"/", Join("lib/gate", common.LoaderFilename), true),
	))

	bin := Group(
		targets.Add(installBinTask(DESTDIR, BINDIR, "bin/gate")),
		targets.Add(installBinTask(DESTDIR, BINDIR, "bin/gate-daemon")),
		targets.Add(installBinTask(DESTDIR, BINDIR, "bin/gate-runtime")),
		targets.Add(installBinTask(DESTDIR, BINDIR, "bin/gate-server")),
	)
	targets.Add(Target("bin", bin))

	targets.Add(Target("all",
		lib,
		bin,

		targets.Add(Target("bash",
			Installation(DESTDIR+"/etc/bash_completion.d/gate", "etc/bash_completion.d/gate.bash", false),
		)),

		targets.Add(Target("systemd",
			installRewriteTask(DESTDIR, SHAREDIR, BINDIR, "systemd/user/gate.service"),

			targets.Add(Target("dbus",
				installRewriteTask(DESTDIR, SHAREDIR, BINDIR, "dbus-1/services/computer.gate.Daemon.service"),
			)),
		)),
	))

	return
}

func installBinTask(DESTDIR, BINDIR, name string) Task {
	task := Installation(DESTDIR+BINDIR+"/", name, true)

	if Exists(name) {
		return TargetDefault(name, task)
	} else {
		return Target(name, task)
	}
}

func installRewriteTask(DESTDIR, SHAREDIR, BINDIR, filename string) Task {
	return Func(func() error {
		b, err := ioutil.ReadFile(Join("share", filename))
		if err != nil {
			return err
		}

		b = bytes.ReplaceAll(b, []byte("/usr/local/bin"), []byte(BINDIR))

		return InstallData(DESTDIR+Join(SHAREDIR, filename), bytes.NewReader(b), false)
	})
}
