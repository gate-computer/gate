// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

const (
	MaxProcesses       = 16384 // Per Executor.
	DefaultCgroupTitle = "gate-runtime"
)

type Cred struct {
	UID uint
	GID uint
}

type Config struct {
	MaxProcesses int
	DaemonSocket string
	ErrorLog     Logger

	// These are applicable if DaemonSocket is not set:
	Container struct{ Cred }
	Executor  struct{ Cred }
	LibDir    string
	Cgroup    CgroupConfig
}

func (c *Config) maxProcesses() int {
	if c.MaxProcesses == 0 {
		return MaxProcesses
	}

	return c.MaxProcesses
}

// CgroupConfig is effective if gate-runtime-container was compiled with cgroup
// support.
type CgroupConfig struct {
	Parent string
	Title  string
}

func (c *CgroupConfig) title() (s string) {
	s = c.Title
	if s == "" {
		s = DefaultCgroupTitle
	}
	return
}
