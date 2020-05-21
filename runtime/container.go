// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"net"
	"os/exec"

	"gate.computer/gate/internal/runtimeapi"
	"gate.computer/gate/internal/sys"
)

func startContainer(config Config) (cmd *exec.Cmd, unixConn *net.UnixConn, err error) {
	binary, err := runtimeapi.ContainerBinary(config.libDir())
	if err != nil {
		return
	}

	controlFile, connFile, err := sys.SocketFilePair(0)
	if err != nil {
		return
	}
	defer controlFile.Close()

	netConn, err := net.FileConn(connFile)
	connFile.Close()
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			netConn.Close()
		}
	}()

	args, err := runtimeapi.ContainerArgs(binary, config.NoNamespaces, config.Container.Cred, config.Executor.Cred, config.Cgroup.title(), config.Cgroup.Parent)
	if err != nil {
		return
	}

	cmd, err = runtimeapi.StartContainer(args, controlFile)
	if err != nil {
		return
	}

	unixConn = netConn.(*net.UnixConn)
	return
}

func dialContainerDaemon(config Config) (conn *net.UnixConn, err error) {
	addr, err := net.ResolveUnixAddr("unix", config.DaemonSocket)
	if err != nil {
		return
	}

	return net.DialUnix(addr.Net, nil, addr)
}
