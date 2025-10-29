// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gate_test

import (
	"errors"
	"os"
	"os/exec"
	"path"
	"runtime"
	"syscall"
	"testing"

	"gate.computer/gate/runtime/container"
	internal "gate.computer/internal/container"
	"gate.computer/internal/sys"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	. "import.name/testing/mustr"
)

var testExecDir = path.Join("../lib", runtime.GOARCH, "gate")

var testNamespaceConfig = container.NamespaceConfig{
	Newuidmap: "newuidmap",
	Newgidmap: "newgidmap",
}

func TestContainerPrivileged(t *testing.T) {
	if os.Getenv("GATE_TEST_PRIVILEGED") == "" {
		t.Skip("skipping privileged container test")
	}

	var ns container.NamespaceConfig
	creds := Must(t, R(internal.ParseCreds(&ns)))
	testContainer(t, ns, creds)
}

func TestContainerNewuidmap(t *testing.T) {
	if os.Getenv("GATE_TEST_CONTAINER") == "" {
		t.Skip("skipping newuidmap container test")
	}

	ns := testNamespaceConfig
	creds := Must(t, R(internal.ParseCreds(&ns)))
	testContainer(t, ns, creds)
}

func TestContainerSingleUID(t *testing.T) {
	if os.Getenv("GATE_TEST_CONTAINER") == "" {
		t.Skip("skipping single-uid container test")
	}

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
	require.NoError(t, err)

	kill := true

	cmd := Must(t, R(internal.Start(otherEnd, config, cred)))
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
	assert.True(t, err == nil || isTermination(err))
}

func isTermination(err error) bool {
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		s := exit.Sys().(syscall.WaitStatus)
		return s.Signaled() && s.Signal() == syscall.SIGTERM
	}
	return false
}
