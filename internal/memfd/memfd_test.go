// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package memfd_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"

	"github.com/tsavola/gate/internal/memfd"
)

func create(t *testing.T, name string, flags memfd.Flags) (fd int) {
	fd, err := memfd.Create(name, flags)
	if err != nil {
		t.Fatal(err)
	}

	if testing.Verbose() {
		cmd := exec.Command("ls", "-l", fmt.Sprintf("/proc/%d/fd/%d", os.Getpid(), fd))
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			t.Fatal(err)
		}
	}

	return
}

func TestEmptyName(t *testing.T) {
	fd := create(t, "", 0)
	defer syscall.Close(fd)
}

func TestName(t *testing.T) {
	fd := create(t, "long name with spaces / etc.", 0)
	defer syscall.Close(fd)
}

func TestCloexec(t *testing.T) {
	fd := create(t, "test", memfd.CLOEXEC)
	defer syscall.Close(fd)

	cmd := exec.Command("readlink", fmt.Sprintf("/proc/self/fd/%d", fd))

	r, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Wait()

	output, err := ioutil.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(string(output), "memfd:") {
		t.Fail()
	}
}

func TestSeal(t *testing.T) {
	fd := create(t, "test", memfd.ALLOW_SEALING)
	defer syscall.Close(fd)

	_, err := memfd.Fcntl(fd, memfd.F_ADD_SEALS, memfd.F_SEAL_SHRINK|memfd.F_SEAL_GROW|memfd.F_SEAL_WRITE)
	if err != nil {
		t.Fatal(err)
	}

	flags, err := memfd.Fcntl(fd, memfd.F_GET_SEALS, 0)
	if err != nil {
		t.Fatal(err)
	}

	if flags != (memfd.F_SEAL_SHRINK | memfd.F_SEAL_GROW | memfd.F_SEAL_WRITE) {
		t.Errorf("0x%x", flags)
	}
}
