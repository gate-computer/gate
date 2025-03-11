// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package container

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"syscall"

	config "gate.computer/gate/runtime/container"
	"gate.computer/internal/container/child"
	"gate.computer/internal/container/common"
	runtimeerrors "gate.computer/internal/error/runtime"
	"golang.org/x/sys/unix"
)

const magicKey = "TNGREHAGVZRPBAGNVARE"

var (
	executable = "/proc/self/exe"
	magicValue = runtime.Version()
)

func Start(controlSocket *os.File, c *config.Config, cred *NamespaceCreds) (*exec.Cmd, error) {
	executorBin, err := openExecutorBinary(c)
	if err != nil {
		return nil, err
	}
	defer executorBin.Close()

	loaderBin, err := openLoaderBinary(c)
	if err != nil {
		return nil, err
	}
	defer loaderBin.Close()

	cgroupDir, err := openDefaultCgroup(&c.Cgroup)
	if err != nil {
		return nil, err
	}
	defer cgroupDir.Close()

	// Intercepted by the init() function below.
	cmd := &exec.Cmd{
		Path:   executable,
		Args:   []string{common.ContainerFilename},
		Env:    append(os.Environ(), magicKey+"="+magicValue),
		Stderr: os.Stderr,
		ExtraFiles: []*os.File{
			controlSocket,
			loaderBin,
			executorBin,
			cgroupDir,
		},
		SysProcAttr: &syscall.SysProcAttr{
			Pdeathsig: unix.SIGKILL,
		},
	}

	if c.Namespace.Disabled {
		cmd.Args = append(cmd.Args, common.ArgNamespaceDisabled)
	} else {
		if !common.Sandbox {
			return nil, errors.New("container namespace must be disabled when sandbox is disabled")
		}
		if c.Namespace.SingleUID {
			cmd.Args = append(cmd.Args, common.ArgSingleUID)
		}
		setupNamespace(cmd.SysProcAttr, &c.Namespace, cred)
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

	if err := configureExecutorCgroup(cmd.Process.Pid, &c.Cgroup); err != nil {
		return nil, err
	}

	if common.Sandbox {
		procOOMScoreAdj := fmt.Sprintf("/proc/%d/oom_score_adj", cmd.Process.Pid)
		if err := os.WriteFile(procOOMScoreAdj, []byte("1000"), 0); err != nil {
			return nil, err
		}

		if !c.Namespace.Disabled && !c.Namespace.SingleUID && isNewidmap(&c.Namespace) {
			if err := configureUserNamespace(cmd.Process.Pid, &c.Namespace, cred); err != nil {
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

func init() {
	// Intercept the self-execution.
	if len(os.Args) > 0 && os.Args[0] == common.ContainerFilename && os.Getenv(magicKey) == magicValue {
		os.Unsetenv(magicKey)
		child.Exec()
	}
}

// Wait for process to exit.  It is requested to exit via the executor API; the
// done channel just tells us if it was expected or not.
func Wait(cmd *exec.Cmd, done <-chan struct{}) error {
	err := cmd.Wait()

	var status *exec.ExitError
	if errors.As(err, &status) && status.Exited() {
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
		return fmt.Errorf("container terminated unexpectedly: %w", err)
	}
}
