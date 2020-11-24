// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

	"gate.computer/gate/internal/bus"
	"gate.computer/gate/server/api"
	webapi "gate.computer/gate/server/web/api"
	dbus "github.com/godbus/dbus/v5"
	"golang.org/x/sys/unix"
	"google.golang.org/protobuf/proto"
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
		usage:  "module [function]",
		detail: moduleUsage,
		parse:  parseCallFlags,
		do: func() {
			if flag.NArg() > 1 {
				c.Function = flag.Arg(1)
			}

			r, w := openStdio()
			rFD := dbus.UnixFD(r.Fd())
			wFD := dbus.UnixFD(w.Fd())

			suspend := newSignalPipe(syscall.SIGQUIT)
			suspendFD := dbus.UnixFD(suspend.Fd())

			debug := openDebugFile()
			debugFD := dbus.UnixFD(debug.Fd())

			var (
				module *os.File
				call   *dbus.Call
			)
			if !(strings.Contains(flag.Arg(0), "/") || strings.Contains(flag.Arg(0), ".")) {
				call = daemonCall("CallKey", flag.Arg(0), c.Function, rFD, wFD, suspendFD, debugFD, c.DebugLog != "", c.InstanceTags, c.ModuleTags, c.Scope)
			} else {
				module = openFile(flag.Arg(0))
				moduleFD := dbus.UnixFD(module.Fd())
				call = daemonCall("CallFile", moduleFD, c.Function, c.Pin, rFD, wFD, suspendFD, debugFD, c.DebugLog != "", c.Scope)
			}
			closeFiles(module, r, w, suspend, debug)

			var (
				instID string
				status = new(api.Status)
			)
			check(call.Store(&instID, &status.State, &status.Cause, &status.Result))

			switch status.State {
			case api.StateSuspended:
				fmt.Fprintln(terminalOr(os.Stderr), instID, statusString(status))

			case api.StateHalted:
				fmt.Fprintln(terminalOr(os.Stderr), instID, statusString(status))
				os.Exit(int(status.Result))

			case api.StateTerminated:
				os.Exit(int(status.Result))

			case api.StateKilled:
				log.Fatal(statusString(status))

			default:
				log.Fatal(instID, statusString(status))
			}
		},
	},

	"debug": {
		usage: "instance [command [offset...]]",
		do: func() {
			debug(func(instID string, req *api.DebugRequest) *api.DebugResponse {
				reqBuf, err := proto.Marshal(req)
				check(err)

				call := daemonCall("Debug", instID, reqBuf)
				var resBuf []byte
				check(call.Store(&resBuf))

				res := new(api.DebugResponse)
				check(proto.Unmarshal(resBuf, res))
				return res
			})
		},
	},

	"delete": {
		usage: "instance",
		do: func() {
			check(daemonCall("Delete", flag.Arg(0)).Store())
		},
	},

	"export": {
		usage: "module [filename]",
		do: func() {
			var filename string
			if flag.NArg() > 1 {
				filename = flag.Arg(1)
			}

			exportLocal(flag.Arg(0), filename)
		},
	},

	"import": {
		usage: "filename [moduletag...]",
		do: func() {
			if tail := flag.Args()[1:]; len(tail) != 0 {
				c.ModuleTags = tail
			}

			var (
				r      *os.File
				w      *os.File
				length int64
				copied chan error
			)

			r = openFile(flag.Arg(0))
			if info, err := r.Stat(); err == nil && info.Mode().IsRegular() {
				length = info.Size()
			} else {
				data, err := ioutil.ReadAll(r)
				r.Close()
				check(err)
				length = int64(len(data))

				r, w, err = os.Pipe()
				check(err)

				copied = make(chan error, 1)
				go func() {
					_, err := w.Write(data)
					copied <- err
				}()
			}

			rFD := dbus.UnixFD(r.Fd())
			call := daemonCall("Upload", rFD, length, "", c.ModuleTags)
			closeFiles(r)

			var progID string
			check(call.Store(&progID))

			if copied != nil {
				check(<-copied)
			}

			fmt.Println(progID)
		},
	},

	"instances": {
		do: func() {
			call := daemonCall("ListInstances")
			var ids []string
			check(call.Store(&ids))

			for _, id := range ids {
				fmt.Printf("%-36s %s\n", id, daemonCallGetInstanceInfo(id))
			}
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

	"kill": {
		usage: "instance",
		do: func() {
			daemonCallInstanceWaiter("Kill", flag.Arg(0))
		},
	},

	"launch": {
		usage:  "module [function [instancetag...]]",
		detail: moduleUsage,
		do: func() {
			if flag.NArg() > 1 {
				c.Function = flag.Arg(1)
				if tail := flag.Args()[2:]; len(tail) != 0 {
					c.InstanceTags = tail
				}
			}

			debug := openDebugFile()
			debugFD := dbus.UnixFD(debug.Fd())

			var (
				module *os.File
				call   *dbus.Call
			)
			if !(strings.Contains(flag.Arg(0), "/") || strings.Contains(flag.Arg(0), ".")) {
				call = daemonCall("LaunchKey", flag.Arg(0), c.Function, c.Suspend, debugFD, c.DebugLog != "", c.InstanceTags, c.ModuleTags, c.Scope)
			} else {
				module = openFile(flag.Arg(0))
				moduleFD := dbus.UnixFD(module.Fd())
				call = daemonCall("LaunchFile", moduleFD, c.Function, c.Pin, c.Suspend, debugFD, c.DebugLog != "", c.InstanceTags, c.ModuleTags, c.Scope)
			}
			closeFiles(module, debug)

			var instance string
			check(call.Store(&instance))

			fmt.Println(instance)
		},
	},

	"modules": {
		do: func() {
			call := daemonCall("ListModules")
			var ids []string
			check(call.Store(&ids))

			for _, id := range ids {
				call := daemonCall("GetModuleInfo", id)
				var tags []string
				check(call.Store(&tags))

				fmt.Println(id, tags)
			}
		},
	},

	"pin": {
		usage: "module [moduletag...]",
		do: func() {
			if tail := flag.Args()[1:]; len(tail) != 0 {
				c.ModuleTags = tail
			}

			check(daemonCall("Pin", flag.Arg(0), c.ModuleTags).Store())
		},
	},

	"pull": {
		usage: "address module",
		do: func() {
			c.address = flag.Arg(0)

			_, resp := doHTTP(nil, webapi.PathKnownModules+flag.Arg(1), nil)
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

			var progID string
			check(call.Store(&progID))

			check(<-copied)
		},
	},

	"push": {
		usage: "address module",
		do: func() {
			c.address = flag.Arg(0)

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
				webapi.ParamAction: []string{webapi.ActionPin},
			}

			doHTTP(req, webapi.PathKnownModules+flag.Arg(1), params)
		},
	},

	"repl": {
		usage: "instance",
		do: func() {
			ir, iw, err := os.Pipe()
			check(err)
			or, ow, err := os.Pipe()
			check(err)

			orFD := dbus.UnixFD(or.Fd())
			iwFD := dbus.UnixFD(iw.Fd())

			call := make(chan *dbus.Call, 1)
			go func() {
				defer close(call)
				call <- daemonCall("IO", flag.Arg(0), orFD, iwFD)
				closeFiles(or, iw)
			}()

			repl(ir, ow)
			ow.Close()
			ir.Close()

			var ok bool
			if c := <-call; c != nil {
				check(c.Store(&ok))
			}

			if !ok {
				os.Exit(1)
			}
		},
	},

	"resume": {
		usage: "instance [function]",
		do: func() {
			if flag.NArg() > 1 {
				c.Function = flag.Arg(1)
			}

			debug := openDebugFile()
			debugFD := dbus.UnixFD(debug.Fd())
			call := daemonCall("Resume", flag.Arg(0), c.Function, debugFD, c.DebugLog != "", c.Scope)
			closeFiles(debug)
			check(call.Store())
		},
	},

	"show": {
		usage: "module",
		do: func() {
			call := daemonCall("GetModuleInfo", flag.Arg(0))
			var tags []string
			check(call.Store(&tags))

			fmt.Println(tags)
		},
	},

	"snapshot": {
		usage: "instance [moduletag...]",
		do: func() {
			if tail := flag.Args()[1:]; len(tail) != 0 {
				c.ModuleTags = tail
			}

			call := daemonCall("Snapshot", flag.Arg(0), c.ModuleTags)
			var progID string
			check(call.Store(&progID))

			fmt.Println(progID)
		},
	},

	"status": {
		usage: "instance",
		do: func() {
			fmt.Println(daemonCallGetInstanceInfo(flag.Arg(0)))
		},
	},

	"suspend": {
		usage: "instance",
		do: func() {
			daemonCallInstanceWaiter("Suspend", flag.Arg(0))
		},
	},

	"unpin": {
		usage: "module",
		do: func() {
			check(daemonCall("Unpin", flag.Arg(0)).Store())
		},
	},

	"update": {
		usage: "instance [instancetag...]",
		do: func() {
			tags := c.InstanceTags
			if tail := flag.Args()[1:]; len(tail) != 0 {
				tags = tail
			}
			if len(tags) == 0 {
				log.Fatal("no tags")
			}

			call := daemonCall("Update", flag.Arg(0), true, tags)
			check(call.Store())
		},
	},

	"wait": {
		usage: "instance",
		do: func() {
			fmt.Println(daemonCallWait(flag.Arg(0)))
		},
	},
}

func exportLocal(module, filename string) {
	download(filename, func() (r io.Reader, moduleLen int64) {
		r, w, err := os.Pipe()
		check(err)

		wFD := dbus.UnixFD(w.Fd())
		call := daemonCall("Download", wFD, module)
		closeFiles(w)

		check(call.Store(&moduleLen))
		return
	})
}

func daemonCallGetInstanceInfo(id string) string {
	call := daemonCall("GetInstanceInfo", id)
	var status = new(api.Status)
	var tags []string
	check(call.Store(&status.State, &status.Cause, &status.Result, &tags))

	return fmt.Sprintf("%s %s", statusString(status), tags)
}

func daemonCallWait(id string) string {
	call := daemonCall("Wait", id)
	status := new(api.Status)
	check(call.Store(&status.State, &status.Cause, &status.Result))

	return statusString(status)
}

func daemonCallInstanceWaiter(method, id string) {
	check(daemonCall(method, id).Store())

	if c.Wait {
		fmt.Println(daemonCallWait(id))
	}
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

func newSignalPipe(signals ...os.Signal) *os.File {
	r, w, err := os.Pipe()
	check(err)

	c := make(chan os.Signal)
	signal.Notify(c, signals...)
	go func() {
		defer w.Close()
		<-c

		// Newline after the ^\ in case the signal was sent via terminal.
		fmt.Fprintln(terminalOr(ioutil.Discard))

		w.Write([]byte{0})
	}()

	return r
}

func openDebugFile() *os.File {
	var name string
	if c.DebugLog == "" {
		name = os.DevNull
	} else {
		name = c.DebugLog
	}
	f, err := os.OpenFile(name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
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

func statusString(s *api.Status) string {
	t := webapi.Status{
		State:  s.GetState().String(),
		Cause:  s.GetCause().String(),
		Result: int(s.GetResult()),
		Error:  s.GetError(),
	}
	if s.GetCause() == 0 {
		t.Cause = ""
	}
	return t.String()
}
