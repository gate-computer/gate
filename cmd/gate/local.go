// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"

	dbus "github.com/godbus/dbus/v5"
	"github.com/tsavola/gate/internal/bus"
	"github.com/tsavola/gate/server"
	"github.com/tsavola/gate/webapi"
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

			// TODO: copy data if TTY
			var (
				rFD = dbus.UnixFD(os.Stdin.Fd())
				wFD = dbus.UnixFD(os.Stdout.Fd())
			)

			f := openDebugFile()
			defer runtime.KeepAlive(f)
			debugFD := dbus.UnixFD(f.Fd())

			var call *dbus.Call
			if module := flag.Arg(0); !strings.Contains(module, "/") {
				call = daemonCall("CallKey", module, c.Function, rFD, wFD, debugFD)
			} else {
				f, err := os.Open(module)
				check(err)
				defer runtime.KeepAlive(f)
				call = daemonCall("CallFile", dbus.UnixFD(f.Fd()), c.Function, c.Ref, rFD, wFD, debugFD)
			}

			var status server.Status
			check(call.Store(&status.State, &status.Cause, &status.Result))

			if status.State != server.StateTerminated || status.Cause != 0 {
				log.Fatal(statusString(status))
			}
			os.Exit(int(status.Result))
		},
	},

	"io": {
		usage: "instance",
		do: func() {
			// TODO: copy data if TTY
			call := daemonCall("IO", flag.Arg(0), dbus.UnixFD(os.Stdin.Fd()), dbus.UnixFD(os.Stdout.Fd()))

			var ok bool
			check(call.Store(&ok))

			if !ok {
				os.Exit(1)
			}
		},
	},

	"launch": {
		usage: "module [function]",
		do: func() {
			if flag.NArg() > 1 {
				c.Function = flag.Arg(1)
			}

			f := openDebugFile()
			defer runtime.KeepAlive(f)
			debugFD := dbus.UnixFD(f.Fd())

			var call *dbus.Call
			if module := flag.Arg(0); !strings.Contains(module, "/") {
				call = daemonCall("LaunchKey", module, c.Function, debugFD)
			} else {
				f, err := os.Open(module)
				check(err)
				defer runtime.KeepAlive(f)
				call = daemonCall("LaunchFile", dbus.UnixFD(f.Fd()), c.Function, c.Ref, debugFD)
			}

			var instance string
			check(call.Store(&instance))

			fmt.Println(instance)
		},
	},

	"wait": {
		usage: "instance",
		do: func() {
			call := daemonCall("Wait", flag.Arg(0))

			var status server.Status
			check(call.Store(&status.State, &status.Cause, &status.Result))

			fmt.Println(statusString(status))
		},
	},
}

func openDebugFile() *os.File {
	var name string
	if c.Debug == "" {
		name = os.DevNull
	} else {
		name = c.Debug
	}
	f, err := os.OpenFile(name, os.O_WRONLY, 0)
	check(err)
	return f
}

func statusString(s server.Status) string {
	return webapi.Status{
		State:  s.State.String(),
		Cause:  s.Cause.String(),
		Result: int(s.Result),
		Error:  s.Error,
		Debug:  s.Debug,
	}.String()
}

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
