// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package container

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path"
	"strings"
	"syscall"

	config "gate.computer/gate/runtime/container"
	"github.com/coreos/go-systemd/v22/dbus"
	"golang.org/x/sys/unix"
)

func configureExecutorCgroup(containerPID int, c *config.CgroupConfig) error {
	if c.Executor == "" {
		return nil
	}

	scope := c.Executor
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
	if c.Parent != "" {
		props = append(props, dbus.PropSlice(c.Parent))
	}

	ctx := context.Background()

	conn, err := dbus.NewSystemdConnectionContext(ctx)
	if err != nil {
		c, err2 := dbus.NewUserConnectionContext(ctx)
		if err2 != nil {
			return fmt.Errorf("D-Bus connection attempts: %v; %w", err, err2)
		}
		conn = c
	}
	defer conn.Close()

	result := make(chan string, 1)

	if _, err := conn.StartTransientUnitContext(ctx, scope, "fail", props, result); err != nil {
		return fmt.Errorf("starting transient systemd unit for container: %w", err)
	}

	if r := <-result; r != "done" {
		return fmt.Errorf("starting transient systemd unit for container: %s", r)
	}

	return nil
}

func openDefaultCgroup(c *config.CgroupConfig) (*os.File, error) {
	if c.Process == "" {
		return os.Open(os.DevNull)
	}

	fd, err := OpenCgroupFD(c.Process)
	if err != nil {
		return nil, err
	}

	return os.NewFile(uintptr(fd), c.Process), nil
}

func OpenCgroupFD(name string) (int, error) {
	dirname := path.Join("/sys/fs/cgroup/unified", name)

	fd, err := syscall.Open(dirname, unix.O_CLOEXEC|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_PATH, 0)
	if err != nil {
		return -1, fmt.Errorf("open directory %s: %w", dirname, err)
	}

	return fd, nil
}
