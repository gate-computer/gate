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

	"gate.computer/gate/runtime/container"
	internal "gate.computer/internal/container"
	"gate.computer/internal/sys"
)

var testExecDir = "../lib/gate"

var testNamespaceConfig = container.NamespaceConfig{
	Newuidmap: "newuidmap",
	Newgidmap: "newgidmap",
}

func TestContainerPrivileged(t *testing.T) {
	if os.Getenv("GATE_TEST_PRIVILEGED") == "" {
		t.SkipNow()
	}

	var ns container.NamespaceConfig
	creds, err := internal.ParseCreds(&ns)
	if err != nil {
		t.Fatal(err)
	}
	testContainer(t, ns, creds)
}

func TestContainerNewuidmap(t *testing.T) {
	ns := testNamespaceConfig
	creds, err := internal.ParseCreds(&ns)
	if err != nil {
		t.Fatal(err)
	}
	testContainer(t, ns, creds)
}

func TestContainerSingleUID(t *testing.T) {
	testContainer(t, container.NamespaceConfig{SingleUID: true}, nil)
}

func TestContainerDisabled(t *testing.T) {
	testContainer(t, container.NamespaceConfig{Disabled: true}, nil)
}

func testContainer(t *testing.T, ns container.NamespaceConfig, cred *internal.NamespaceCreds) {
	t.Helper()

	config := &container.Config{
		Namespace: ns,
		ExecDir:   testExecDir,
	}

	otherEnd, thisEnd, err := sys.SocketFilePair(0)
	if err != nil {
		t.Fatal(err)
	}

	kill := true

	cmd, err := internal.Start(otherEnd, config, cred)
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
