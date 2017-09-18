// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package run

const (
	DefaultMaxProcs    = 32767 - 1 // practical maximum (minus init process)
	DefaultCgroupTitle = "gate-executor"
)

type Cred struct {
	Uid uint
	Gid uint
}

type Config struct {
	MaxProcs     int
	DaemonSocket string
	ErrorLog     Logger

	// FileLimiter limits the number of simultaneous file descriptors used by
	// runtimes.  When the limit is reached, calls block.  By default there is
	// no limit.  This only applies to the Go program; file descriptor usage of
	// child processes are not accounted for.
	//
	// A Runtime instance itself requires some file descriptors during its
	// initialization (unless DaemonSocket is used), of which one is required
	// for the lifetime of the runtime (regardless of DaemonSocket usage).
	//
	// An Image requires one file descriptor.
	//
	// Process initialization requires 4 file descriptors (or 6 if debug output
	// is enabled), of which 2 (or 3) are required for the lifetime of the
	// process.
	FileLimiter *FileLimiter

	// The rest are only applicable if DaemonSocket is not set:
	ContainerCred Cred
	ExecutorCred  Cred
	LibDir        string

	// These have no effect if container was compiled without cgroup support:
	CgroupParent string
	CgroupTitle  string
}

func (c *Config) maxProcs() int64 {
	if c.MaxProcs == 0 {
		return DefaultMaxProcs
	} else {
		return int64(c.MaxProcs)
	}
}

func (c *Config) cgroupTitle() (s string) {
	s = c.CgroupTitle
	if s == "" {
		s = DefaultCgroupTitle
	}
	return
}
