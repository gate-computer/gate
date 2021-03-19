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

	"gate.computer/gate/internal/container"
	"gate.computer/gate/runtime"
	"github.com/coreos/go-systemd/v22/activation"
	"github.com/coreos/go-systemd/v22/daemon"
	"github.com/tsavola/confi"
)

type Config struct {
	Runtime runtime.Config

	Log struct {
		Syslog bool
	}
}

var c = new(Config)

func main() {
	c.Runtime = runtime.DefaultConfig

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

	creds, err := container.ParseCreds(&c.Runtime.Container.Namespace.User)
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
			go handle(client, conn, creds, errLog, infoLog)
		} else {
			errLog.Print(err)
		}
	}
}

func handle(client uint64, conn *net.UnixConn, creds *container.NamespaceCreds, errLog, infoLog *log.Logger) {
	defer func() {
		if conn != nil {
			conn.Close()
		}
	}()

	infoLog.Printf("%d: connection", client)

	connFile, err := conn.File()
	if err != nil {
		errLog.Printf("%d: %v", client, err)
		return
	}
	defer func() {
		if connFile != nil {
			connFile.Close()
		}
	}()

	conn.Close()
	conn = nil

	cmd, err := container.Start(connFile, &c.Runtime.Container, creds)
	if err != nil {
		errLog.Printf("%d: %v", client, err)
		return
	}

	connFile.Close()
	connFile = nil

	if err := container.Wait(cmd, nil); err == nil {
		infoLog.Printf("%d: container terminated", client)
	} else {
		errLog.Printf("%d: %v", client, err)
	}
}
