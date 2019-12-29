// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"flag"
	"log"
	"os"
	"strings"

	dbus "github.com/godbus/dbus/v5"
	"github.com/tsavola/gate/internal/bus"
	"github.com/tsavola/gate/server"
)

var daemon dbus.BusObject

func daemonCall(method string, args ...interface{}) *dbus.Call {
	if daemon == nil {
		conn, err := dbus.SessionBus()
		check(err)

		daemon = conn.Object(bus.DaemonIface, bus.DaemonPath)
	}

	return daemon.Call(bus.DaemonIface+"."+method, 0, args...)
}

var localCommands = map[string]command{
	"call": {
		usage: "module [function]",
		do: func() {
			if flag.NArg() > 1 {
				c.Function = flag.Arg(1)
			}

			var (
				rFD     = dbus.UnixFD(os.Stdin.Fd())
				wFD     = dbus.UnixFD(os.Stdout.Fd())
				debugFD dbus.UnixFD
			)
			switch c.Debug {
			case "":
				f, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
				check(err)
				debugFD = dbus.UnixFD(f.Fd())

			case "stderr":
				debugFD = dbus.UnixFD(os.Stderr.Fd())

			default:
				panic(errors.New("unsupported debug option"))
			}

			if true {
				f, err := os.OpenFile(os.DevNull, os.O_RDONLY, 0)
				check(err)
				rFD = dbus.UnixFD(f.Fd())
			}

			var call *dbus.Call
			if module := flag.Arg(0); !strings.Contains(module, "/") {
				call = daemonCall("CallKey", module, c.Function, rFD, wFD, debugFD)
			} else {
				f, err := os.Open(module)
				check(err)
				call = daemonCall("CallFile", dbus.UnixFD(f.Fd()), c.Function, c.Ref, rFD, wFD, debugFD)
			}

			var (
				state  server.State
				cause  server.Cause
				result int
			)
			check(call.Store(&state, &cause, &result))

			if state != server.StateTerminated || cause != 0 {
				log.Fatal(state, cause)
			}
			os.Exit(result)
		},
	},
}

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
