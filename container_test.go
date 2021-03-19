// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gate_test

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"testing"

	"gate.computer/gate/internal/container"
	"gate.computer/gate/internal/sys"
)

const testLibDir = "lib/gate/runtime"

var testUserNamespaceConfig = container.UserNamespaceConfig{
	Newuidmap: "/usr/bin/newuidmap",
	Newgidmap: "/usr/bin/newgidmap",
}

func TestContainerPrivileged(t *testing.T) {
	if os.Getenv("GATE_TEST_PRIVILEGED") == "" {
		t.SkipNow()
	}

	var u container.UserNamespaceConfig
	creds, err := container.ParseCreds(&u)
	if err != nil {
		t.Fatal(err)
	}
	testContainer(t, container.NamespaceConfig{User: u}, creds)
}

func TestContainerNewuidmap(t *testing.T) {
	u := testUserNamespaceConfig
	creds, err := container.ParseCreds(&u)
	if err != nil {
		t.Fatal(err)
	}
	testContainer(t, container.NamespaceConfig{User: u}, creds)
}

func TestContainerSingleUID(t *testing.T) {
	u := container.UserNamespaceConfig{SingleUID: true}
	testContainer(t, container.NamespaceConfig{User: u}, nil)
}

func TestContainerDisabled(t *testing.T) {
	testContainer(t, container.NamespaceConfig{Disabled: true}, nil)
}

func testContainer(t *testing.T, ns container.NamespaceConfig, cred *container.NamespaceCreds) {
	t.Helper()

	config := &container.ContainerConfig{
		LibDir:    testLibDir,
		Namespace: ns,
	}

	otherEnd, thisEnd, err := sys.SocketFilePair(0)
	if err != nil {
		t.Fatal(err)
	}

	kill := true

	cmd, err := container.Start(otherEnd, config, cred)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if kill {
			cmd.Process.Kill()
			cmd.Wait()
		}
	}()

	otherEnd.Close()

	// No operation.

	thisEnd.Close()

	err = cmd.Wait()
	kill = false
	if err != nil && !isTermination(err) {
		t.Error(err)
	}
}

func isTermination(err error) bool {
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		s := exit.Sys().(syscall.WaitStatus)
		return s.Signaled() && s.Signal() == syscall.SIGTERM
	}
	return false
}
