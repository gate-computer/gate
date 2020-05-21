// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"os"
	"path"

	"gate.computer/gate/internal/runtimeapi"
)

const MaxProcs = 16384 // Per Executor.

var DefaultLibDir string = func() string {
	var parent string
	if filename, err := os.Executable(); err == nil {
		parent = path.Join(path.Dir(filename), "..")
	}
	return path.Join(parent, "lib", "gate", "runtime")
}()

type Cred = runtimeapi.Cred

type Config struct {
	MaxProcs     int
	ConnFile     *os.File
	DaemonSocket string // Applicable if ConnFile is not set.
	ErrorLog     Logger

	// These are applicable if ConnFile and DaemonSocket are not set:
	NoNamespaces bool
	Container    struct{ Cred }
	Executor     struct{ Cred }
	LibDir       string
	Cgroup       CgroupConfig
}

func (c Config) maxProcs() int {
	if c.MaxProcs == 0 {
		return MaxProcs
	}
	return c.MaxProcs
}

func (c Config) libDir() string {
	if c.LibDir == "" {
		return DefaultLibDir
	}
	return c.LibDir
}

const DefaultCgroupTitle = "gate-runtime"

// CgroupConfig is effective if gate-runtime-container was compiled with cgroup
// support.
type CgroupConfig struct {
	Parent string
	Title  string
}

func (c CgroupConfig) title() (s string) {
	s = c.Title
	if s == "" {
		s = DefaultCgroupTitle
	}
	return
}

var DefaultConfig = Config{
	MaxProcs: MaxProcs,
	LibDir:   DefaultLibDir,
	Cgroup: CgroupConfig{
		Title: DefaultCgroupTitle,
	},
}
