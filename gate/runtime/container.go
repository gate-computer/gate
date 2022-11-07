// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"net"
	"os/exec"

	"gate.computer/gate/runtime/container"
	internal "gate.computer/internal/container"
	"gate.computer/internal/sys"
)

func startContainer(c *container.Config) (cmd *exec.Cmd, unixConn *net.UnixConn, err error) {
	creds, err := internal.ParseCreds(&c.Namespace)
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

	cmd, err = internal.Start(controlFile, c, creds)
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
