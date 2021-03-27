// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package container

import (
	"os"
	"syscall"

	"gate.computer/gate/internal/container/common"
	config "gate.computer/gate/runtime/container"
	"golang.org/x/sys/unix"
)

var ExecutablePath = "/proc/self/exe"

func newSysProcAttr(ns *config.NamespaceConfig, cred *NamespaceCreds) *syscall.SysProcAttr {
	attr := &syscall.SysProcAttr{
		Setsid:    true,
		Pdeathsig: unix.SIGKILL,
	}

	if !ns.Disabled {
		attr.Cloneflags = unix.CLONE_NEWCGROUP | unix.CLONE_NEWIPC | unix.CLONE_NEWNET | unix.CLONE_NEWNS | unix.CLONE_NEWPID | unix.CLONE_NEWUSER | unix.CLONE_NEWUTS

		attr.AmbientCaps = []uintptr{
			unix.CAP_DAC_OVERRIDE,
			unix.CAP_SETGID,
			unix.CAP_SETUID,
			unix.CAP_SYS_ADMIN,
		}

		if ns.SingleUID {
			attr.UidMappings = []syscall.SysProcIDMap{
				{ContainerID: common.ContainerCred, HostID: os.Getuid(), Size: 1},
			}
			attr.GidMappings = []syscall.SysProcIDMap{
				{ContainerID: common.ContainerCred, HostID: os.Getgid(), Size: 1},
			}
		} else if !isNewidmap(ns) {
			attr.UidMappings = []syscall.SysProcIDMap{
				{ContainerID: common.ContainerCred, HostID: cred.Container.UID, Size: 1},
				{ContainerID: common.ExecutorCred, HostID: cred.Executor.UID, Size: 1},
			}
			attr.GidMappings = []syscall.SysProcIDMap{
				{ContainerID: common.ContainerCred, HostID: cred.Container.GID, Size: 1},
				{ContainerID: common.ExecutorCred, HostID: cred.Executor.GID, Size: 1},
			}
			attr.GidMappingsEnableSetgroups = true
		}
	}

	return attr
}
