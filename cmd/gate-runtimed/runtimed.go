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
	"path"

	"github.com/coreos/go-systemd/v22/activation"
	"github.com/coreos/go-systemd/v22/daemon"
	"github.com/tsavola/confi"
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
	c.Runtime.LibDir = "lib/gate/runtime"
	c.Runtime.Cgroup.Title = runtime.DefaultCgroupTitle

	flag.Var(confi.FileReader(c), "f", "read a configuration file")
	flag.Var(confi.Assigner(c), "o", "set a configuration option (path.to.key=value)")
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

	binary, err := runtimeapi.ContainerBinary(c.Runtime.LibDir)
	if err != nil {
		critLog.Fatal(err)
	}

	containerArgs, err := runtimeapi.ContainerArgs(binary, c.Runtime.NoNamespaces, c.Runtime.Container.Cred, c.Runtime.Executor.Cred, c.Runtime.Cgroup.Title, c.Runtime.Cgroup.Parent)
	if err != nil {
		critLog.Fatal(err)
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

	if _, err := daemon.SdNotify(true, daemon.SdNotifyReady); err != nil {
		critLog.Fatal(err)
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

	cmd, err := runtimeapi.StartContainer(containerArgs, connFile)
	connFile.Close()
	if err != nil {
		errLog.Printf("%d: %v", client, err)
		return
	}

	if err := runtimeapi.WaitForContainer(cmd, nil); err == nil {
		infoLog.Printf("%d: container terminated", client)
	} else {
		errLog.Printf("%d: %v", client, err)
	}
}
