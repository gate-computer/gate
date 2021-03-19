// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"os"

	"gate.computer/gate/internal/container"
)

const (
	MaxProcs           = 16384 // Per Executor.
	DefaultSubuid      = container.FallbackSubuid
	DefaultSubgid      = container.FallbackSubgid
	DefaultCgroupTitle = container.FallbackCgroupTitle
)

var DefaultLibDir = container.FallbackLibDir

type Cred = container.Cred
type ContainerConfig = container.ContainerConfig
type NamespaceConfig = container.NamespaceConfig
type UserNamespaceConfig = container.UserNamespaceConfig
type CgroupConfig = container.CgroupConfig

type Config struct {
	MaxProcs     int
	ConnFile     *os.File
	DaemonSocket string          // Applicable if ConnFile is not set.
	Container    ContainerConfig // Applicable if ConnFile and DaemonSocket are not set.
	ErrorLog     Logger
}

func (c *Config) maxprocs() int {
	if c.MaxProcs == 0 {
		return MaxProcs
	}
	return c.MaxProcs
}

var DefaultConfig = Config{
	MaxProcs: MaxProcs,
	Container: ContainerConfig{
		LibDir: DefaultLibDir,
		Namespace: NamespaceConfig{
			User: UserNamespaceConfig{
				Subuid: DefaultSubuid,
				Subgid: DefaultSubgid,
			},
		},
		Cgroup: CgroupConfig{
			Title: DefaultCgroupTitle,
		},
	},
}
