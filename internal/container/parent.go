// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package container

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"syscall"

	"gate.computer/gate/internal/container/common"
	runtimeerrors "gate.computer/gate/internal/error/runtime"
)

var FallbackLibDir = path.Join(binParent(), "lib", "gate", "runtime")

func binParent() string {
	if filename, err := os.Executable(); err == nil {
		return path.Join(path.Dir(filename), "..")
	}
	return ""
}

type ContainerConfig struct {
	LibDir    string
	Namespace NamespaceConfig
	Cgroup    CgroupConfig
}

func (c *ContainerConfig) binaryPath(name string) (string, error) {
	dir := c.LibDir
	if dir == "" {
		dir = FallbackLibDir
	}
	return filepath.Abs(path.Join(dir, name))
}

func Start(controlSocket *os.File, c *ContainerConfig, cred *NamespaceCreds) (*exec.Cmd, error) {
	containerPath, err := c.binaryPath(common.ContainerName)
	if err != nil {
		return nil, err
	}

	executorPath, err := c.binaryPath(common.ExecutorName)
	if err != nil {
		return nil, err
	}

	loaderPath, err := c.binaryPath(common.LoaderName)
	if err != nil {
		return nil, err
	}

	executorBin, err := openPath(executorPath, syscall.O_NOFOLLOW)
	if err != nil {
		return nil, err
	}
	defer executorBin.Close()

	loaderBin, err := openPath(loaderPath, syscall.O_NOFOLLOW)
	if err != nil {
		return nil, err
	}
	defer loaderBin.Close()

	cmd := &exec.Cmd{
		Path:   containerPath,
		Args:   []string{common.ContainerName},
		Env:    []string{},
		Dir:    "/",
		Stderr: os.Stderr,
		ExtraFiles: []*os.File{
			controlSocket,
			loaderBin,
			executorBin,
		},
		SysProcAttr: newSysProcAttr(&c.Namespace, cred),
	}

	if c.Namespace.Disabled {
		cmd.Args = append(cmd.Args, common.ArgNamespaceDisabled)
	} else if c.Namespace.User.SingleUID {
		cmd.Args = append(cmd.Args, common.ArgSingleUID)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	defer func() {
		if stdin != nil {
			stdin.Close()
		}
	}()

	kill := true

	if err := cmd.Start(); err != nil {
		return nil, err
	}
	defer func() {
		if kill {
			cmd.Process.Kill()
			cmd.Wait()
		}
	}()

	if err := configureCgroup(cmd.Process.Pid, &c.Cgroup); err != nil {
		return nil, err
	}

	if common.Sandbox {
		if err := writeOOMScoreAdj(cmd.Process.Pid); err != nil {
			return nil, err
		}

		if !c.Namespace.Disabled && !c.Namespace.User.SingleUID && !c.Namespace.User.selfservice() {
			if err := configureUserNamespace(cmd.Process.Pid, &c.Namespace.User, cred); err != nil {
				return nil, err
			}
		}
	}

	err = stdin.Close()
	stdin = nil
	if err != nil {
		return nil, err
	}

	kill = false
	return cmd, nil
}

// Wait for process to exit.  It is requested to exit via the executor API; the
// done channel just tells us if it was expected or not.
func Wait(cmd *exec.Cmd, done <-chan struct{}) error {
	err := cmd.Wait()

	if status, ok := err.(*exec.ExitError); ok && status.Exited() {
		switch code := status.Sys().(syscall.WaitStatus).ExitStatus(); code {
		case 0:
			err = nil

		case 1:
			err = errors.New("(message should have been written to stderr)")

		default:
			err = runtimeerrors.ExecutorError(code)
		}
	}

	select {
	case <-done:
		return err

	default:
		if err == nil {
			err = errors.New("(no error code)")
		}
		return fmt.Errorf("container terminated unexpectedly: %v", err)
	}
}
