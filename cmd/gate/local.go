// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	runtimedebug "runtime/debug"
	"sort"
	"strings"
	"syscall"

	"gate.computer/gate/server/api"
	"gate.computer/gate/web"
	"gate.computer/internal/bus"
	"github.com/godbus/dbus/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/term"
	"google.golang.org/protobuf/proto"
)

var daemon dbus.BusObject

func daemonCall(method string, args ...any) *dbus.Call {
	tracer := otel.GetTracerProvider().Tracer("gate/cmd/gate")
	_, span := tracer.Start(context.Background(), method, trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()

	c := span.SpanContext()
	traceID := c.TraceID()
	spanID := c.SpanID()

	slog.Debug("call",
		"method", method,
		"trace", traceID,
		"span", spanID,
	)

	args = append([]any{traceID[:], spanID[:]}, args...)

	if daemon == nil {
		conn := must(dbus.SessionBus())
		daemon = conn.Object(bus.DaemonIface, bus.DaemonPath)
	}

	return daemon.Call(bus.DaemonIface+"."+method, 0, args...)
}

var (
	persistInstance bool
	debugMore       bool
)

func parseLocalCallFlags() {
	registerRunFlags()
	debug := flag.Bool("d", c.DebugLog == ShortcutDebugLog, "write debug log to stderr")
	flag.BoolVar(&persistInstance, "p", persistInstance, "keep instance after it stops")
	flag.BoolVar(&debugMore, "D", debugMore, "write debug log, keep instance and dump stack")
	flag.Parse()

	if *debug || debugMore {
		c.DebugLog = ShortcutDebugLog
	}
	if debugMore {
		persistInstance = true
	}
}

var localCommands = map[string]command{
	"call": {
		usage:    "module [function]",
		detail:   moduleUsage,
		discover: discoverLocalScope,
		parse:    parseLocalCallFlags,
		do: func() {
			module := flag.Arg(0)
			if flag.NArg() > 1 {
				c.Function = flag.Arg(1)
			}

			suspend := newSignalPipe(syscall.SIGQUIT)
			suspendFD := dbus.UnixFD(suspend.Fd())

			r, w := openStdio()
			rFD := dbus.UnixFD(r.Fd())
			wFD := dbus.UnixFD(w.Fd())

			debug := openDebugFile()
			debugFD := dbus.UnixFD(debug.Fd())

			var (
				moduleFile *os.File
				call       *dbus.Call
			)
			if !(strings.Contains(module, "/") || strings.Contains(module, ".")) {
				call = daemonCall("Call", module, c.Function, c.InstanceTags, c.Scope, !persistInstance, suspendFD, rFD, wFD, debugFD, c.DebugLog != "")
			} else {
				moduleFile = must(os.Open(module))
				moduleFD := dbus.UnixFD(moduleFile.Fd())
				call = daemonCall("CallFile", moduleFD, c.Pin, c.ModuleTags, c.Function, c.InstanceTags, c.Scope, !persistInstance, suspendFD, rFD, wFD, debugFD, c.DebugLog != "")
			}
			closeFiles(suspend, r, w, debug, moduleFile)

			var (
				instanceID string
				status     = new(api.Status)
			)
			z.Check(call.Store(&instanceID, &status.State, &status.Cause, &status.Result))

			if persistInstance {
				fmt.Fprintln(terminalOr(os.Stderr), instanceID, statusString(status))
			} else {
				fmt.Fprintln(terminalOr(os.Stderr), statusString(status))
			}

			if debugMore {
				switch status.State {
				case api.StateHalted, api.StateTerminated:
				default:
					reqBuf := must(proto.Marshal(&api.DebugRequest{Op: api.DebugOpReadStack}))

					call := daemonCall("DebugInstance", instanceID, reqBuf)
					var resBuf []byte
					z.Check(call.Store(&resBuf))

					res := new(api.DebugResponse)
					z.Check(proto.Unmarshal(resBuf, res))

					fmt.Fprintln(terminalOr(os.Stderr), "Call stack:")
					debugBacktrace(res)
				}
			}

			code := 1
			switch status.State {
			case api.StateSuspended:
				code = 0
			case api.StateHalted, api.StateTerminated:
				code = int(status.Result)
			}
			os.Exit(code)
		},
	},

	"debug": {
		usage: "instance [command [offset...]]",
		do: func() {
			debug(func(instID string, req *api.DebugRequest) *api.DebugResponse {
				reqBuf := must(proto.Marshal(req))

				call := daemonCall("DebugInstance", instID, reqBuf)
				var resBuf []byte
				z.Check(call.Store(&resBuf))

				res := new(api.DebugResponse)
				z.Check(proto.Unmarshal(resBuf, res))
				return res
			})
		},
	},

	"delete": {
		usage: "instance",
		do: func() {
			z.Check(daemonCall("DeleteInstance", flag.Arg(0)).Store())
		},
	},

	"export": {
		usage: "module [filename]",
		do: func() {
			var filename string
			if flag.NArg() > 1 {
				filename = flag.Arg(1)
			}

			exportLocalModule(flag.Arg(0), filename)
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

			r = must(os.Open(flag.Arg(0)))
			if info, err := r.Stat(); err == nil && info.Mode().IsRegular() {
				length = info.Size()
			} else {
				data, err := io.ReadAll(r)
				r.Close()
				z.Check(err)
				length = int64(len(data))

				r, w = must2(os.Pipe())

				copied = make(chan error, 1)
				go func() {
					_, err := w.Write(data)
					copied <- err
				}()
			}

			rFD := dbus.UnixFD(r.Fd())
			call := daemonCall("UploadModule", rFD, length, "", c.ModuleTags)
			closeFiles(r)

			var moduleID string
			z.Check(call.Store(&moduleID))

			if copied != nil {
				z.Check(<-copied)
			}

			fmt.Println(moduleID)
		},
	},

	"instances": {
		do: func() {
			call := daemonCall("ListInstances")
			var ids []string
			z.Check(call.Store(&ids))
			sort.Strings(ids)

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

			call := daemonCall("ConnectInstance", flag.Arg(0), rFD, wFD)
			closeFiles(r, w)

			var ok bool
			z.Check(call.Store(&ok))

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
		usage:    "module [function [instancetag...]]",
		detail:   moduleUsage,
		discover: discoverLocalScope,
		parse:    parseLaunchFlags,
		do: func() {
			module := flag.Arg(0)
			if flag.NArg() > 1 {
				c.Function = flag.Arg(1)
				if tail := flag.Args()[2:]; len(tail) != 0 {
					c.InstanceTags = tail
				}
			}

			debug := openDebugFile()
			debugFD := dbus.UnixFD(debug.Fd())

			var (
				moduleFile *os.File
				call       *dbus.Call
			)
			if !(strings.Contains(module, "/") || strings.Contains(module, ".")) {
				call = daemonCall("Launch", module, c.Function, c.Suspend, c.InstanceTags, c.Scope, debugFD, c.DebugLog != "")
			} else {
				moduleFile = must(os.Open(module))
				moduleFD := dbus.UnixFD(moduleFile.Fd())
				call = daemonCall("LaunchFile", moduleFD, c.Pin, c.ModuleTags, c.Function, c.Suspend, c.InstanceTags, c.Scope, debugFD, c.DebugLog != "")
			}
			closeFiles(debug, moduleFile)

			var instanceID string
			z.Check(call.Store(&instanceID))

			fmt.Println(instanceID)
		},
	},

	"modules": {
		do: func() {
			call := daemonCall("ListModules")
			var ids []string
			z.Check(call.Store(&ids))
			sort.Strings(ids)

			for _, id := range ids {
				call := daemonCall("GetModuleInfo", id)
				var tags []string
				z.Check(call.Store(&tags))

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

			z.Check(daemonCall("PinModule", flag.Arg(0), c.ModuleTags).Store())
		},
	},

	"pull": {
		usage: "address module",
		do: func() {
			c.address = flag.Arg(0)

			_, resp := doHTTP(nil, web.PathKnownModules+flag.Arg(1), nil)
			if resp.ContentLength < 0 {
				fatal("server did not specify content length")
			}

			r, w := must2(os.Pipe())

			copied := make(chan error, 1)
			go func() {
				defer w.Close()
				_, err := io.Copy(w, resp.Body)
				copied <- err
			}()

			rFD := dbus.UnixFD(r.Fd())
			call := daemonCall("UploadModule", rFD, resp.ContentLength, flag.Arg(1), c.ModuleTags)
			closeFiles(r)

			var moduleID string
			z.Check(call.Store(&moduleID))

			z.Check(<-copied)
		},
	},

	"push": {
		usage: "address module",
		do: func() {
			c.address = flag.Arg(0)

			r, w := must2(os.Pipe())

			wFD := dbus.UnixFD(w.Fd())
			call := daemonCall("DownloadModule", wFD, flag.Arg(1))
			closeFiles(w)

			var moduleLen int64
			z.Check(call.Store(&moduleLen))

			req := &http.Request{
				Method: http.MethodPut,
				Header: http.Header{
					web.HeaderContentType: []string{web.ContentTypeWebAssembly},
				},
				Body:          r,
				ContentLength: moduleLen,
			}
			params := url.Values{
				web.ParamAction: []string{web.ActionPin},
			}

			doHTTP(req, web.PathKnownModules+flag.Arg(1), params)
		},
	},

	"repl": {
		usage: "instance",
		do: func() {
			ir, iw := must2(os.Pipe())
			or, ow := must2(os.Pipe())

			orFD := dbus.UnixFD(or.Fd())
			iwFD := dbus.UnixFD(iw.Fd())

			call := make(chan *dbus.Call, 1)
			go func() {
				defer close(call)
				call <- daemonCall("ConnectInstance", flag.Arg(0), orFD, iwFD)
				closeFiles(or, iw)
			}()

			repl(ir, ow)
			ow.Close()
			ir.Close()

			var ok bool
			if c := <-call; c != nil {
				z.Check(c.Store(&ok))
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
			call := daemonCall("ResumeInstance", flag.Arg(0), c.Function, c.Scope, debugFD, c.DebugLog != "")
			closeFiles(debug)
			z.Check(call.Store())
		},
	},

	"show": {
		usage: "module",
		do: func() {
			call := daemonCall("GetModuleInfo", flag.Arg(0))
			var tags []string
			z.Check(call.Store(&tags))

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
			var moduleID string
			z.Check(call.Store(&moduleID))

			fmt.Println(moduleID)
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
			daemonCallInstanceWaiter("SuspendInstance", flag.Arg(0))
		},
	},

	"unpin": {
		usage: "module",
		do: func() {
			z.Check(daemonCall("UnpinModule", flag.Arg(0)).Store())
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
				fatal("no tags")
			}

			call := daemonCall("UpdateInstance", flag.Arg(0), true, tags)
			z.Check(call.Store())
		},
	},

	"version": {
		do: func() {
			var version string
			if info, ok := runtimedebug.ReadBuildInfo(); ok {
				version = info.Main.Version
			}
			if version != "" {
				fmt.Println("Gate client version:", version)
			} else {
				fmt.Println("Gate client version is unknown")
			}
			fmt.Println("Go compiler version:", runtime.Version())
		},
	},

	"wait": {
		usage: "instance",
		do: func() {
			fmt.Println(daemonCallWaitInstance(flag.Arg(0)))
		},
	},
}

func discoverLocalScope(w io.Writer) {
	fmt.Fprintln(w)

	call := daemonCall("GetScope")
	var scope []string
	z.Check(call.Store(&scope))

	printScope(w, scope)
}

func exportLocalModule(moduleID, filename string) {
	download(filename, func() (r io.Reader, moduleLen int64) {
		r, w := must2(os.Pipe())

		wFD := dbus.UnixFD(w.Fd())
		call := daemonCall("DownloadModule", wFD, moduleID)
		closeFiles(w)

		z.Check(call.Store(&moduleLen))
		return
	})
}

func daemonCallGetInstanceInfo(id string) string {
	call := daemonCall("GetInstanceInfo", id)
	var (
		status = new(api.Status)
		tags   []string
	)
	z.Check(call.Store(&status.State, &status.Cause, &status.Result, &tags))

	return fmt.Sprintf("%s %s", statusString(status), tags)
}

func daemonCallWaitInstance(id string) string {
	call := daemonCall("WaitInstance", id)
	status := new(api.Status)
	z.Check(call.Store(&status.State, &status.Cause, &status.Result))

	return statusString(status)
}

func daemonCallInstanceWaiter(method, id string) {
	z.Check(daemonCall(method, id).Store())

	if c.Wait {
		fmt.Println(daemonCallWaitInstance(id))
	}
}

func openStdio() (r, w *os.File) {
	r = os.Stdin
	if term.IsTerminal(int(r.Fd())) {
		r = copyStdin()
	}

	w = os.Stdout
	if term.IsTerminal(int(w.Fd())) {
		w = copyStdout()
	}

	return
}

func copyStdin() *os.File {
	r, w := must2(os.Pipe())

	go func() {
		defer w.Close()
		io.Copy(w, os.Stdin)
	}()

	return r
}

func copyStdout() *os.File {
	r, w := must2(os.Pipe())

	go func() {
		defer r.Close()
		io.Copy(os.Stdout, r)
	}()

	return w
}

func newSignalPipe(signals ...os.Signal) *os.File {
	r, w := must2(os.Pipe())

	c := make(chan os.Signal, 1)
	signal.Notify(c, signals...)
	go func() {
		defer w.Close()
		<-c

		// Newline after the ^\ in case the signal was sent via terminal.
		fmt.Fprintln(terminalOr(io.Discard))

		w.Write([]byte{0})
	}()

	return r
}

func openDebugFile() *os.File {
	switch c.DebugLog {
	case "/dev/stderr":
		return os.Stderr

	case "":
		return must(os.OpenFile(os.DevNull, os.O_WRONLY, 0))

	default:
		return must(os.OpenFile(c.DebugLog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644))
	}
}

func closeFiles(files ...*os.File) {
	for _, f := range files {
		if f == os.Stderr {
			continue
		}

		// Avoid the object from being garbage-collected while its file
		// descriptor is being handled directly.
		runtime.KeepAlive(f)

		if f != nil {
			f.Close()
		}
	}
}

func statusString(s *api.Status) string {
	t := web.Status{
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
