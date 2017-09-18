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
	"github.com/tsavola/gate/internal/cred"
	"github.com/tsavola/gate/run"
)

func main() {
	var (
		config = run.Config{
			LibDir:      "lib",
			CgroupTitle: run.DefaultCgroupTitle,
		}
		syslogging = false
	)

	flag.StringVar(&config.DaemonSocket, "listen", config.DaemonSocket, "unix socket")
	flag.UintVar(&config.ContainerCred.Uid, "container-uid", config.ContainerCred.Uid, "user id for bootstrapping executor")
	flag.UintVar(&config.ContainerCred.Gid, "container-gid", config.ContainerCred.Gid, "group id for bootstrapping executor")
	flag.UintVar(&config.ExecutorCred.Uid, "executor-uid", config.ExecutorCred.Uid, "user id for executing code")
	flag.UintVar(&config.ExecutorCred.Gid, "executor-gid", config.ExecutorCred.Gid, "group id for executing code")
	flag.StringVar(&config.LibDir, "libdir", config.LibDir, "path")
	flag.StringVar(&config.CgroupParent, "cgroup-parent", config.CgroupParent, "slice")
	flag.StringVar(&config.CgroupTitle, "cgroup-title", config.CgroupTitle, "prefix of dynamic name")
	flag.BoolVar(&syslogging, "syslog", syslogging, "send log messages to syslog instead of stderr")

	flag.Parse()

	var (
		critLog *log.Logger
		errLog  *log.Logger
		infoLog *log.Logger
	)

	if syslogging {
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

	containerPath, err := filepath.Abs(path.Join(config.LibDir, "gate-container"))
	if err != nil {
		return
	}

	creds, err := cred.Parse(config.ContainerCred.Uid, config.ContainerCred.Gid, config.ExecutorCred.Uid, config.ExecutorCred.Gid)
	if err != nil {
		return
	}

	containerArgs := []string{
		containerPath,
		creds[0],
		creds[1],
		creds[2],
		creds[3],
		config.CgroupTitle,
		config.CgroupParent,
	}

	listeners, err := activation.Listeners(true)
	if err != nil {
		critLog.Fatal(err)
	}

	var listener *net.UnixListener

	switch len(listeners) {
	case 0:
		if config.DaemonSocket == "" {
			critLog.Fatal("-listen option or socket activation required")
		}

		addr, err := net.ResolveUnixAddr("unix", config.DaemonSocket)
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

	case 1:
		if config.DaemonSocket != "" {
			critLog.Fatal("-listen option used with socket activation")
		}

		listener = listeners[0].(*net.UnixListener)

	default:
		critLog.Fatal("too many sockets to activate")
	}

	for client := uint64(0); ; client++ {
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
