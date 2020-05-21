// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtimeapi

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"syscall"

	"gate.computer/gate/internal/cred"
	runtimeerrors "gate.computer/gate/internal/error/runtime"
)

const (
	version           = "0"
	containerFilename = "gate-runtime-container." + version
)

type Cred struct {
	UID uint
	GID uint
}

func ContainerBinary(libdir string) (binary string, err error) {
	return filepath.Abs(path.Join(libdir, containerFilename))
}

func ContainerArgs(binary string, noNamespaces bool, containerCred, executorCred Cred, cgroupTitle, cgroupParent string,
) (args []string, err error) {
	creds, err := cred.Parse(containerCred.UID, containerCred.GID, executorCred.UID, executorCred.GID)
	if err != nil {
		return
	}

	flags := 0
	if noNamespaces {
		flags |= 1
	}

	args = []string{
		binary,
		strconv.Itoa(flags),
		creds[0],
		creds[1],
		creds[2],
		creds[3],
		cgroupTitle,
		cgroupParent,
	}
	return
}

func StartContainer(args []string, control *os.File) (cmd *exec.Cmd, err error) {
	cmd = &exec.Cmd{
		Path:   args[0],
		Args:   args,
		Dir:    "/",
		Stderr: os.Stderr,
		ExtraFiles: []*os.File{
			control, // GATE_CONTROL_FD
		},
		SysProcAttr: &syscall.SysProcAttr{
			Pdeathsig: syscall.SIGKILL,
		},
	}

	err = cmd.Start()
	if err != nil {
		return
	}

	return
}

// WaitForContainer process to exit.  It is requested to exit via the executor
// API; the done channel just tells us if it was expected or not.
func WaitForContainer(cmd *exec.Cmd, done <-chan struct{}) error {
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
