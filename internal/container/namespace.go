// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package container

import (
	"os"
	"os/exec"
	"strconv"

	"gate.computer/gate/internal/container/common"
)

const (
	FallbackSubuid = "/etc/subuid"
	FallbackSubgid = "/etc/subgid"
)

type NamespaceConfig struct {
	// Don't create new Linux namespaces.  The container doesn't contain; the
	// child processes can "see" host resources.  (Other sandboxing features
	// may still be in effect.)
	Disabled bool

	// Linux user namespace configuration.
	User UserNamespaceConfig
}

type UserNamespaceConfig struct {
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

func (c *UserNamespaceConfig) subuid() string {
	if c.Subuid == "" {
		return FallbackSubuid
	}
	return c.Subuid
}

func (c *UserNamespaceConfig) subgid() string {
	if c.Subgid == "" {
		return FallbackSubgid
	}
	return c.Subgid
}

func (c *UserNamespaceConfig) selfservice() bool {
	return c.Newuidmap == "" && c.Newgidmap == ""
}

// configureUserNamespace with external tools.
func configureUserNamespace(pid int, c *UserNamespaceConfig, cred *NamespaceCreds) error {
	if err := writeIdMap(c.Newuidmap, pid, os.Getuid(), cred.Container.UID, cred.Executor.UID); err != nil {
		return err
	}

	if err := writeIdMap(c.Newgidmap, pid, os.Getgid(), cred.Container.GID, cred.Executor.GID); err != nil {
		return err
	}

	return nil
}

func writeIdMap(binary string, pid, initial, container, executor int) error {
	cmd := exec.Cmd{
		Path: binary,
		Args: []string{
			binary,
			strconv.Itoa(pid),
			// Inside, Outside, Count,
			strconv.Itoa(common.ContainerCred), strconv.Itoa(container), "1",
			strconv.Itoa(common.ExecutorCred), strconv.Itoa(executor), "1",
		},
		Stderr: os.Stderr,
	}

	return cmd.Run()
}
