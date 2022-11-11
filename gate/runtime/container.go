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

func startContainer(c *container.Config) (*exec.Cmd, *net.UnixConn, error) {
	creds, err := internal.ParseCreds(&c.Namespace)
	if err != nil {
		return nil, nil, err
	}

	controlFile, connFile, err := sys.SocketFilePair(0)
	if err != nil {
		return nil, nil, err
	}
	defer controlFile.Close()

	netConn, err := net.FileConn(connFile)
	connFile.Close()
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		if netConn != nil {
			netConn.Close()
		}
	}()

	cmd, err := internal.Start(controlFile, c, creds)
	if err != nil {
		return nil, nil, err
	}

	unixConn := netConn.(*net.UnixConn)
	netConn = nil
	return cmd, unixConn, nil
}

func dialContainerDaemon(path string) (*net.UnixConn, error) {
	addr, err := net.ResolveUnixAddr("unix", path)
	if err != nil {
		return nil, err
	}

	return net.DialUnix(addr.Net, nil, addr)
}
