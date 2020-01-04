// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"

	dbus "github.com/godbus/dbus/v5"
	"github.com/tsavola/gate/internal/bus"
	"github.com/tsavola/gate/server"
	"github.com/tsavola/gate/webapi"
	"golang.org/x/sys/unix"
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

			r, w := openStdio()
			rFD := dbus.UnixFD(r.Fd())
			wFD := dbus.UnixFD(w.Fd())

			debug := openDebugFile()
			debugFD := dbus.UnixFD(debug.Fd())

			var (
				module *os.File
				call   *dbus.Call
			)
			if !strings.Contains(flag.Arg(0), "/") {
				call = daemonCall("CallKey", flag.Arg(0), c.Function, rFD, wFD, debugFD, c.Debug)
			} else {
				module = openFile(flag.Arg(0))
				moduleFD := dbus.UnixFD(module.Fd())
				call = daemonCall("CallFile", moduleFD, c.Function, c.Ref, rFD, wFD, debugFD, c.Debug)
			}
			closeFiles(module, r, w, debug)

			var status server.Status
			check(call.Store(&status.State, &status.Cause, &status.Result))

			if status.State != server.StateTerminated || status.Cause != 0 {
				log.Fatal(statusString(status))
			}
			os.Exit(int(status.Result))
		},
	},

	"delete": {
		usage: "instance",
		do: func() {
			daemonCallInstance("Delete")
		},
	},

	"io": {
		usage: "instance",
		do: func() {
			r, w := openStdio()
			rFD := dbus.UnixFD(r.Fd())
			wFD := dbus.UnixFD(w.Fd())

			call := daemonCall("IO", flag.Arg(0), rFD, wFD)
			closeFiles(r, w)

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

			debug := openDebugFile()
			debugFD := dbus.UnixFD(debug.Fd())

			var (
				module *os.File
				call   *dbus.Call
			)
			if !strings.Contains(flag.Arg(0), "/") {
				call = daemonCall("LaunchKey", flag.Arg(0), c.Function, debugFD, c.Debug)
			} else {
				module = openFile(flag.Arg(0))
				moduleFD := dbus.UnixFD(module.Fd())
				call = daemonCall("LaunchFile", moduleFD, c.Function, c.Ref, debugFD, c.Debug)
			}
			closeFiles(module, debug)

			var instance string
			check(call.Store(&instance))

			fmt.Println(instance)
		},
	},

	"pull": {
		usage: "address module",
		do: func() {
			c.Address = flag.Arg(0)

			_, resp := doHTTP(nil, webapi.PathModuleRefs+flag.Arg(1), nil)
			if resp.ContentLength < 0 {
				log.Fatal("server did not specify content length")
			}

			r, w, err := os.Pipe()
			check(err)

			copied := make(chan error, 1)
			go func() {
				defer w.Close()
				_, err := io.Copy(w, resp.Body)
				copied <- err
			}()

			rFD := dbus.UnixFD(r.Fd())
			call := daemonCall("Upload", rFD, resp.ContentLength, flag.Arg(1))
			closeFiles(r)
			check(call.Store())

			check(<-copied)
		},
	},

	"push": {
		usage: "address module",
		do: func() {
			c.Address = flag.Arg(0)

			r, w, err := os.Pipe()
			check(err)

			wFD := dbus.UnixFD(w.Fd())
			call := daemonCall("Download", wFD, flag.Arg(1))
			closeFiles(w)

			var moduleLen int64
			check(call.Store(&moduleLen))

			req := &http.Request{
				Method: http.MethodPut,
				Header: http.Header{
					webapi.HeaderContentType: []string{webapi.ContentTypeWebAssembly},
				},
				Body:          r,
				ContentLength: moduleLen,
			}
			params := url.Values{
				webapi.ParamAction: []string{webapi.ActionRef},
			}

			doHTTP(req, webapi.PathModuleRefs+flag.Arg(1), params)
		},
	},

	"resume": {
		usage: "instance",
		do: func() {
			debug := openDebugFile()
			debugFD := dbus.UnixFD(debug.Fd())

			call := daemonCall("Resume", flag.Arg(0), debugFD, c.Debug)
			closeFiles(debug)
			check(call.Store())
		},
	},

	"status": {
		usage: "instance",
		do: func() {
			status := daemonCallInstance("Status")
			fmt.Println(statusString(status))
		},
	},

	"suspend": {
		usage: "instance",
		do: func() {
			status := daemonCallInstance("Suspend")
			fmt.Println(statusString(status))
		},
	},

	"wait": {
		usage: "instance",
		do: func() {
			status := daemonCallInstance("Wait")
			fmt.Println(statusString(status))
		},
	},
}

func daemonCallInstance(name string) (status server.Status) {
	call := daemonCall(name, flag.Arg(0))
	check(call.Store(&status.State, &status.Cause, &status.Result))
	return
}

func openStdio() (r *os.File, w *os.File) {
	r = os.Stdin
	if _, err := unix.IoctlGetTermios(int(r.Fd()), unix.TCGETS); err == nil {
		r = copyStdin()
	}

	w = os.Stdout
	if _, err := unix.IoctlGetTermios(int(w.Fd()), unix.TCGETS); err == nil {
		w = copyStdout()
	}

	return
}

func copyStdin() *os.File {
	r, w, err := os.Pipe()
	check(err)

	go func() {
		defer w.Close()
		io.Copy(w, os.Stdin)
	}()

	return r
}

func copyStdout() *os.File {
	r, w, err := os.Pipe()
	check(err)

	go func() {
		defer r.Close()
		io.Copy(os.Stdout, r)
	}()

	return w
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

func closeFiles(files ...*os.File) {
	for _, f := range files {
		// Avoid the object from being garbage-collected while its file
		// descriptor is being handled directly.
		runtime.KeepAlive(f)

		if f != nil {
			f.Close()
		}
	}
}

func statusString(s server.Status) string {
	t := webapi.Status{
		State:  s.State.String(),
		Cause:  s.Cause.String(),
		Result: int(s.Result),
		Error:  s.Error,
		Debug:  s.Debug,
	}
	if s.State == 0 {
		t.State = ""
	}
	if s.Cause == 0 {
		t.Cause = ""
	}
	return t.String()
}
