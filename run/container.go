// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package run

import (
	"log"
	"net"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"syscall"

	"github.com/tsavola/gate/internal/cred"
)

func startContainer(config *Config) (cmd *exec.Cmd, unixConn *net.UnixConn, err error) {
	err = cred.ValidateIds("user", syscall.Getuid(), 2, config.ContainerCred.Uid, config.ExecutorCred.Uid)
	if err != nil {
		return
	}

	err = cred.ValidateIds("group", syscall.Getgid(), 2, config.ContainerCred.Gid, config.ExecutorCred.Gid, config.CommonGid)
	if err != nil {
		return
	}

	containerPath, err := filepath.Abs(path.Join(config.LibDir, "container"))
	if err != nil {
		return
	}

	fdPair, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC, 0)
	if err != nil {
		return
	}

	controlFile := os.NewFile(uintptr(fdPair[0]), "executor (peer)")
	defer controlFile.Close()

	connFile := os.NewFile(uintptr(fdPair[1]), "executor")
	defer connFile.Close()

	netConn, err := net.FileConn(connFile)
	if err != nil {
		return
	}

	cmd = &exec.Cmd{
		Path: containerPath,
		Args: []string{
			containerPath,
			cred.FormatId(config.CommonGid),
			cred.FormatId(config.ContainerCred.Uid),
			cred.FormatId(config.ContainerCred.Gid),
			cred.FormatId(config.ExecutorCred.Uid),
			cred.FormatId(config.ExecutorCred.Gid),
			config.cgroupTitle(),
			config.CgroupParent,
		},
		Dir:    "/",
		Stderr: os.Stderr,
		ExtraFiles: []*os.File{
			controlFile,
		},
		SysProcAttr: &syscall.SysProcAttr{
			Pdeathsig: syscall.SIGKILL,
		},
	}

	err = cmd.Start()
	if err != nil {
		netConn.Close()
		return
	}

	unixConn = netConn.(*net.UnixConn)
	return
}

// Wait for the container process to exit.  It is requested to exit via the
// executor API; the done channel just tells us if it was expected or not.
func containerWaiter(cmd *exec.Cmd, done <-chan struct{}) {
	err := cmd.Wait()

	select {
	case <-done:
		if exit, ok := err.(*exec.ExitError); ok && exit.Success() {
			// expected and clean
		} else {
			log.Printf("container process exited with an error: %v", err)
		}

	default:
		log.Printf("container process exited unexpectedly: %v", err)
	}
}

func dialContainerDaemon(config *Config) (conn *net.UnixConn, err error) {
	addr, err := net.ResolveUnixAddr("unix", config.DaemonSocket)
	if err != nil {
		return
	}

	conn, err = net.DialUnix(addr.Net, nil, addr)
	return
}
