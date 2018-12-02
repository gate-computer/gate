// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"syscall"

	"github.com/tsavola/gate/internal/cred"
	internal "github.com/tsavola/gate/internal/error/runtime"
	"github.com/tsavola/gate/internal/runtimeapi"
)

func startContainer(ctx context.Context, config *Config,
) (cmd *exec.Cmd, unixConn *net.UnixConn, err error) {
	containerPath, err := filepath.Abs(path.Join(config.LibDir, runtimeapi.ContainerFilename))
	if err != nil {
		return
	}

	creds, err := cred.Parse(config.Container.UID, config.Container.GID, config.Executor.UID, config.Executor.GID)
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

	cmd = &exec.Cmd{
		Path: containerPath,
		Args: []string{
			containerPath,
			creds[0],
			creds[1],
			creds[2],
			creds[3],
			config.Cgroup.title(),
			config.Cgroup.Parent,
		},
		Dir:    "/",
		Stderr: os.Stderr,
		ExtraFiles: []*os.File{
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
	return
}

// Wait for the container process to exit.  It is requested to exit via the
// executor API; the done channel just tells us if it was expected or not.
func containerWaiter(cmd *exec.Cmd, done <-chan struct{}, errorLog Logger) {
	err := cmd.Wait()

	if status, ok := err.(*exec.ExitError); ok && status.Exited() {
		switch code := status.Sys().(syscall.WaitStatus).ExitStatus(); code {
		case 0:
			err = nil

		case 1:
			err = errors.New("(message should have been written to stderr)")

		default:
			err = internal.ExecutorError(code)
		}
	}

	select {
	case <-done:
		if err != nil {
			errorLog.Printf("%v", err)
		}

	default:
		if err == nil {
			err = errors.New("(no error code)")
		}
		errorLog.Printf("container terminated unexpectedly: %v", err)
	}
}

func dialContainerDaemon(ctx context.Context, config *Config) (conn *net.UnixConn, err error) {
	addr, err := net.ResolveUnixAddr("unix", config.DaemonSocket)
	if err != nil {
		return
	}

	return net.DialUnix(addr.Net, nil, addr)
}
