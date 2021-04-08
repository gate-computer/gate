// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package container

// ExecDir may be preset via linker option.  Otherwise it is deduced from
// executable path at run time.  Meaningful only if Go build tag gateexecdir
// was specified.
var ExecDir string

type Config struct {
	Namespace NamespaceConfig
	Cgroup    CgroupConfig
	ExecDir   string // Effective only if Go build tag gateexecdir was specified.
}

// Cred specifies a user id and a group id.  A zero value means unspecified
// (not root).
type Cred struct {
	UID int
	GID int
}

var (
	Subuid = "/etc/subuid"
	Subgid = "/etc/subgid"
)

type NamespaceConfig struct {
	// Don't create new Linux namespaces.  The container doesn't contain; the
	// child processes can "see" host resources.  (Other sandboxing features
	// may still be in effect.)
	Disabled bool

	// If true, configure the user namespace with only the current host user
	// and group id mapped inside the namespace.  If unprivileged user
	// namespace creation is allowed by kernel configuration, no privileges are
	// needed for configuring the namespace.  However, all resources and
	// processes inside the namespace will have same ownership.
	//
	// If false, attempt to configure the user namespace with multiple user and
	// group ids.  Resources (such as mounts and directories) will be owned by
	// a different user than the one executing the processes.
	SingleUID bool

	// The host ids mapped inside the container when multiple user/group ids
	// are being used.  Container credentials are used when initializing the
	// container's resources, and executor credentials are used to run the
	// executor process and its children.
	Container Cred
	Executor  Cred

	// When using multiple user and group ids, but container and executor
	// credentials are not explicitly provided (they are zero), these text
	// files are used to discover appropriate id ranges.  See subuid(5).
	Subuid string
	Subgid string

	// Capable (setuid root) binaries for configuring the user namespace with
	// multiple user and group ids.  If not provided, the current process must
	// have sufficient privileges.  See newuidmap(1).
	Newuidmap string
	Newgidmap string
}

var CgroupTitle = "gate-runtime"

type CgroupConfig struct {
	// Don't configure cgroups.
	Disabled bool

	// Name of the parent slice.  Default is to keep the current parent.
	Parent string

	// Prefix for the created slice's name.
	Title string
}

var DefaultConfig = Config{
	Namespace: NamespaceConfig{
		Subuid: Subuid,
		Subgid: Subgid,
	},
	Cgroup: CgroupConfig{
		Title: CgroupTitle,
	},
	ExecDir: ExecDir,
}