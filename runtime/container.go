// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"net"
	"os/exec"

	"gate.computer/gate/internal/container"
	"gate.computer/gate/internal/sys"
)

func startContainer(config *ContainerConfig) (cmd *exec.Cmd, unixConn *net.UnixConn, err error) {
	creds, err := container.ParseCreds(&config.Namespace.User)
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
		if netConn != nil {
			netConn.Close()
		}
	}()

	cmd, err = container.Start(controlFile, config, creds)
	if err != nil {
		return
	}

	unixConn = netConn.(*net.UnixConn)
	netConn = nil
	return
}

func dialContainerDaemon(path string) (conn *net.UnixConn, err error) {
	addr, err := net.ResolveUnixAddr("unix", path)
	if err != nil {
		return
	}

	return net.DialUnix(addr.Net, nil, addr)
}
