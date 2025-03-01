// Copyright (c) 2022 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"os"

	"gate.computer/internal/container/common"
	. "import.name/make"
)

func main() { Main(targets, "") }

// These are set via -ldflags by make.go.
var (
	PREFIX     string
	LIBEXECDIR string
)

func targets() (targets Tasks) {
	var (
		DESTDIR  = Getvar("DESTDIR", "")
		BINDIR   = Getvar("BINDIR", Join(PREFIX, "bin"))
		SHAREDIR = Getvar("SHAREDIR", Join(PREFIX, "share"))
		ETCDIR   = Getvar("ETCDIR", "/etc")
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
			installRewriteTask(DESTDIR, BINDIR, SHAREDIR, "share", "systemd/system/gate-runtime.service"),
			installRewriteTask(DESTDIR, BINDIR, SHAREDIR, "share", "systemd/system/gate-runtime.socket"),
			installRewriteTask(DESTDIR, BINDIR, SHAREDIR, "share", "systemd/system/gate-server.service"),
			installRewriteTask(DESTDIR, BINDIR, SHAREDIR, "share", "systemd/user/gate-daemon.service"),

			targets.Add(Target("dbus",
				installRewriteTask(DESTDIR, BINDIR, SHAREDIR, "share", "dbus-1/services/computer.gate.Daemon.service"),
			)),
		)),

		targets.Add(Target("apparmor",
			installRewriteTask(DESTDIR, BINDIR, ETCDIR, "etc", "apparmor.d/gate-daemon"),
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

func installRewriteTask(DESTDIR, BINDIR, targetdir, sourcedir, filename string) Task {
	return Func(func() error {
		b, err := os.ReadFile(Join(sourcedir, filename))
		if err != nil {
			return err
		}

		b = bytes.ReplaceAll(b, []byte("/usr/local/bin"), []byte(BINDIR))

		return InstallData(DESTDIR+Join(targetdir, filename), bytes.NewReader(b), false)
	})
}
