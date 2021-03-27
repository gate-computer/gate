// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package container

import (
	"os"
	"os/exec"
	"strconv"

	"gate.computer/gate/internal/container/common"
	config "gate.computer/gate/runtime/container"
)

func getSubuid(c *config.NamespaceConfig) string {
	if c.Subuid == "" {
		return config.Subuid
	}
	return c.Subuid
}

func getSubgid(c *config.NamespaceConfig) string {
	if c.Subgid == "" {
		return config.Subgid
	}
	return c.Subgid
}

func isNewidmap(c *config.NamespaceConfig) bool {
	return c.Newuidmap != "" || c.Newgidmap != ""
}

// configureUserNamespace with external tools.
func configureUserNamespace(pid int, c *config.NamespaceConfig, cred *NamespaceCreds) error {
	if err := writeIdMap(c.Newuidmap, pid, os.Getuid(), cred.Container.UID, cred.Executor.UID); err != nil {
		return err
	}

	if err := writeIdMap(c.Newgidmap, pid, os.Getgid(), cred.Container.GID, cred.Executor.GID); err != nil {
		return err
	}

	return nil
}

func writeIdMap(binary string, pid, initial, container, executor int) error {
	cmd := exec.Command(binary, strconv.Itoa(pid),
		// Inside, Outside, Count,
		strconv.Itoa(common.ContainerCred), strconv.Itoa(container), "1",
		strconv.Itoa(common.ExecutorCred), strconv.Itoa(executor), "1",
	)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
