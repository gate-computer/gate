// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"log"
	"log/syslog"
	"net"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"syscall"

	"github.com/coreos/go-systemd/activation"
	"github.com/tsavola/confi"
	"github.com/tsavola/gate/internal/cred"
	"github.com/tsavola/gate/internal/runtimeapi"
	"github.com/tsavola/gate/runtime"
)

type Config struct {
	Runtime runtime.Config

	Log struct {
		Syslog bool
	}
}

func main() {
	c := new(Config)
	c.Runtime.MaxProcs = runtime.DefaultMaxProcs
	c.Runtime.LibDir = "lib/gate/runtime"
	c.Runtime.Cgroup.Title = runtime.DefaultCgroupTitle

	flag.Var(confi.FileReader(c), "f", "read TOML configuration file")
	flag.Var(confi.Assigner(c), "c", "set a configuration key (path.to.key=value)")
	flag.Usage = confi.FlagUsage(nil, c)
	flag.Parse()

	var (
		critLog *log.Logger
		errLog  *log.Logger
		infoLog *log.Logger
	)
	if c.Log.Syslog {
		tag := path.Base(os.Args[0])

		w, err := syslog.New(syslog.LOG_CRIT, tag)
		if err != nil {
			log.Fatal(err)
		}
		critLog = log.New(w, "", 0)

		w, err = syslog.New(syslog.LOG_ERR, tag)
		if err != nil {
			critLog.Fatal(err)
		}
		errLog = log.New(w, "", 0)

		w, err = syslog.New(syslog.LOG_INFO, tag)
		if err != nil {
			critLog.Fatal(err)
		}
		infoLog = log.New(w, "", 0)
	} else {
		critLog = log.New(os.Stderr, "", 0)
		errLog = critLog
		infoLog = critLog
	}

	containerPath, err := filepath.Abs(path.Join(c.Runtime.LibDir, runtimeapi.ContainerFilename))
	if err != nil {
		return
	}

	creds, err := cred.Parse(c.Runtime.Container.UID, c.Runtime.Container.GID, c.Runtime.Executor.UID, c.Runtime.Executor.GID)
	if err != nil {
		return
	}

	containerArgs := []string{
		containerPath,
		creds[0],
		creds[1],
		creds[2],
		creds[3],
		c.Runtime.Cgroup.Title,
		c.Runtime.Cgroup.Parent,
	}

	listeners, err := activation.Listeners()
	if err != nil {
		critLog.Fatal(err)
	}

	var listener *net.UnixListener

	switch {
	case len(listeners) == 0 && c.Runtime.DaemonSocket != "":
		addr, err := net.ResolveUnixAddr("unix", c.Runtime.DaemonSocket)
		if err != nil {
			critLog.Fatal(err)
		}

		if info, err := os.Lstat(addr.Name); err == nil {
			if info.Mode()&os.ModeSocket != 0 {
				os.Remove(addr.Name)
			}
		}

		listener, err = net.ListenUnix(addr.Net, addr)
		if err != nil {
			critLog.Fatal(err)
		}

	case len(listeners) == 1 && c.Runtime.DaemonSocket == "":
		listener = listeners[0].(*net.UnixListener)

	case len(listeners) > 1:
		critLog.Fatal("too many sockets to activate")

	default:
		critLog.Fatal("need either runtime.daemonsocket setting or socket activation")
	}

	var client uint64

	for {
		client++

		conn, err := listener.AcceptUnix()
		if err == nil {
			go handle(client, conn, containerArgs, errLog, infoLog)
		} else {
			errLog.Print(err)
		}
	}
}

func handle(client uint64, conn *net.UnixConn, containerArgs []string, errLog, infoLog *log.Logger) {
	infoLog.Printf("%d: connection", client)

	connFile, err := conn.File()
	conn.Close()
	if err != nil {
		errLog.Printf("%d: %v", client, err)
		return
	}

	cmd := exec.Cmd{
		Path:   containerArgs[0],
		Args:   containerArgs,
		Dir:    "/",
		Stderr: os.Stderr,
		ExtraFiles: []*os.File{
			connFile, // GATE_CONTROL_FD
		},
		SysProcAttr: &syscall.SysProcAttr{
			Pdeathsig: syscall.SIGKILL,
		},
	}

	err = cmd.Start()
	connFile.Close()
	if err != nil {
		errLog.Printf("%d: %v", client, err)
		return
	}

	err = cmd.Wait()
	if exit, ok := err.(*exec.ExitError); ok && exit.Success() {
		infoLog.Printf("%d: %v", client, err)
	} else {
		errLog.Printf("%d: %v", client, err)
	}
}
