// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package container

import (
	"crypto/rand"
	"fmt"
	"os"
	"path"
	"strings"
	"syscall"

	config "gate.computer/gate/runtime/container"
	"github.com/coreos/go-systemd/v22/dbus"
)

func configureExecutorCgroup(containerPID int, c *config.CgroupConfig) error {
	if c.ExecutorScope == "" {
		return nil
	}

	scope := c.ExecutorScope
	if !strings.HasSuffix(scope, ".scope") {
		id := make([]byte, 4)
		if _, err := rand.Read(id); err != nil {
			return fmt.Errorf("random: %w", err)
		}
		scope = fmt.Sprintf("%s-%x.scope", scope, id)
	}

	props := []dbus.Property{
		dbus.PropPids(uint32(containerPID)),
	}
	if c.ParentSlice != "" {
		props = append(props, dbus.PropSlice(c.ParentSlice))
	}

	conn, err := dbus.NewSystemdConnection()
	if err != nil {
		c, err2 := dbus.NewUserConnection()
		if err2 != nil {
			return fmt.Errorf("D-Bus connection attempts: %v; %w", err, err2)
		}
		conn = c
	}
	defer conn.Close()

	result := make(chan string, 1)

	if _, err := conn.StartTransientUnit(scope, "fail", props, result); err != nil {
		return fmt.Errorf("starting transient systemd unit for container: %w", err)
	}

	if r := <-result; r != "done" {
		return fmt.Errorf("starting transient systemd unit for container: %s", r)
	}

	return nil
}

func openLoaderCgroupDir(c *config.CgroupConfig) (*os.File, error) {
	if c.LoaderSlice == "" {
		return os.Open(os.DevNull)
	}

	p := path.Join("/sys/fs/cgroup/unified", c.LoaderSlice)
	return openPath(p, syscall.O_DIRECTORY|syscall.O_NOFOLLOW)
}
