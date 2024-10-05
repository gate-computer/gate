// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"flag"
	"log"
	"log/slog"
	"net"
	"os"

	"gate.computer/gate/runtime/container"
	internal "gate.computer/internal/container"
	"gate.computer/internal/logging"
	"github.com/coreos/go-systemd/v22/activation"
	"github.com/coreos/go-systemd/v22/daemon"
	"import.name/confi"
)

type Config struct {
	Runtime struct {
		DaemonSocket string
		Container    container.Config
	}

	Log struct {
		Journal bool
	}
}

var c = new(Config)

func Main() {
	c.Runtime.Container = container.DefaultConfig

	flag.Var(confi.FileReader(c), "f", "read a configuration file")
	flag.Var(confi.Assigner(c), "o", "set a configuration option (path.to.key=value)")
	flag.Usage = confi.FlagUsage(nil, c)
	flag.Parse()

	if c.Log.Journal {
		log.SetFlags(0)
	}
	log, err := logging.Init(c.Log.Journal)
	if err != nil {
		log.Error("journal initialization failed", "error", err)
		os.Exit(1)
	}

	creds, err := internal.ParseCreds(&c.Runtime.Container.Namespace)
	if err != nil {
		log.Error("credentials parsing failed", "error", err)
		os.Exit(1)
	}

	listeners, err := activation.Listeners()
	if err != nil {
		log.Error("listener discovery failed", "error", err)
		os.Exit(1)
	}

	var listener *net.UnixListener

	switch {
	case len(listeners) == 0 && c.Runtime.DaemonSocket != "":
		addr, err := net.ResolveUnixAddr("unix", c.Runtime.DaemonSocket)
		if err != nil {
			log.Error("daemon socket resolution failed", "error", err)
			os.Exit(1)
		}

		if info, err := os.Lstat(addr.Name); err == nil {
			if info.Mode()&os.ModeSocket != 0 {
				os.Remove(addr.Name)
			}
		}

		listener, err = net.ListenUnix(addr.Net, addr)
		if err != nil {
			log.Error("daemon socket listening failed", "error", err)
			os.Exit(1)
		}

	case len(listeners) == 1 && c.Runtime.DaemonSocket == "":
		listener = listeners[0].(*net.UnixListener)

	case len(listeners) > 1:
		log.Error("too many sockets to activate")
		os.Exit(1)

	default:
		log.Error("need either runtime.daemonsocket setting or socket activation")
		os.Exit(1)
	}

	if _, err := daemon.SdNotify(true, daemon.SdNotifyReady); err != nil {
		log.Error("systemd notification failed", "error", err)
		os.Exit(1)
	}

	var client uint64

	for {
		client++

		conn, err := listener.AcceptUnix()
		if err == nil {
			go handle(conn, creds, log.With("client", client))
		} else {
			log.Error("accept error", "error", err)
		}
	}
}

func handle(conn *net.UnixConn, creds *internal.NamespaceCreds, log *slog.Logger) {
	defer func() {
		if conn != nil {
			conn.Close()
		}
	}()

	log.Info("connection")

	connFile, err := conn.File()
	if err != nil {
		log.Error("connection file error", "error", err)
		return
	}
	defer func() {
		if connFile != nil {
			connFile.Close()
		}
	}()

	conn.Close()
	conn = nil

	cmd, err := internal.Start(connFile, &c.Runtime.Container, creds)
	if err != nil {
		log.Error("container start failed", "error", err)
		return
	}

	connFile.Close()
	connFile = nil

	if err := internal.Wait(cmd, nil); err == nil {
		log.Info("container terminated")
	} else {
		log.Error("container wait failed", "error", err)
	}
}
