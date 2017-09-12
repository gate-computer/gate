// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package run

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"syscall"

	"github.com/tsavola/gate/internal/cred"
)

func startContainer(ctx context.Context, limiter FileLimiter, config *Config) (cmd *exec.Cmd, unixConn *net.UnixConn, err error) {
	containerPath, err := filepath.Abs(path.Join(config.LibDir, "gate-container"))
	if err != nil {
		return
	}

	// Acquire files before cred.Parse() because it may allocate temporary file
	// descriptors (one at a time).

	numFiles := 3 + 5 // Trying to guess how many file descriptors cmd.Start() uses
	err = limiter.acquire(ctx, numFiles)
	if err != nil {
		return
	}
	defer func() {
		limiter.release(numFiles)
	}()

	creds, err := cred.Parse(config.ContainerCred.Uid, config.ContainerCred.Gid, config.ExecutorCred.Uid, config.ExecutorCred.Gid)
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
	netConn, err := net.FileConn(connFile)
	connFile.Close()
	if err != nil {
		return
	}

	nullFile, err := os.Open(os.DevNull)
	if err != nil {
		return
	}
	defer nullFile.Close()

	cmd = &exec.Cmd{
		Path: containerPath,
		Args: []string{
			containerPath,
			creds[0],
			creds[1],
			creds[2],
			creds[3],
			config.cgroupTitle(),
			config.CgroupParent,
		},
		Dir:    "/",
		Stderr: os.Stderr,
		ExtraFiles: []*os.File{
			nullFile,    // GATE_NULL_FD
			controlFile, // GATE_CONTROL_FD
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
	numFiles-- // Keep unixConn
	return
}

// Wait for the container process to exit.  It is requested to exit via the
// executor API; the done channel just tells us if it was expected or not.
func containerWaiter(cmd *exec.Cmd, done <-chan struct{}, errorLog Logger) {
	err := cmd.Wait()

	select {
	case <-done:
		if exit, ok := err.(*exec.ExitError); ok && exit.Success() {
			// expected and clean
		} else {
			errorLog.Printf("container process exited with an error: %v", err)
		}

	default:
		errorLog.Printf("container process exited unexpectedly: %v", err)
	}
}

func dialContainerDaemon(ctx context.Context, limiter FileLimiter, config *Config) (conn *net.UnixConn, err error) {
	addr, err := net.ResolveUnixAddr("unix", config.DaemonSocket)
	if err != nil {
		return
	}

	numFiles := 1
	err = limiter.acquire(ctx, numFiles)
	if err != nil {
		return
	}
	defer func() {
		limiter.release(numFiles)
	}()

	conn, err = net.DialUnix(addr.Net, nil, addr)
	if err != nil {
		return
	}

	numFiles-- // Keep conn
	return
}
