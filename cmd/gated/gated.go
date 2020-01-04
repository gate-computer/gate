// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"os"
	"os/signal"
	"path"
	goruntime "runtime"
	"sort"
	"syscall"

	"github.com/coreos/go-systemd/v22/daemon"
	dbus "github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/tsavola/confi"
	"github.com/tsavola/gate/image"
	"github.com/tsavola/gate/internal/bus"
	"github.com/tsavola/gate/principal"
	gateruntime "github.com/tsavola/gate/runtime"
	"github.com/tsavola/gate/server"
	"github.com/tsavola/gate/service"
	"github.com/tsavola/gate/service/catalog"
	"github.com/tsavola/gate/service/origin"
	"github.com/tsavola/gate/service/plugin"
	"github.com/tsavola/wag/compile"
)

type debugKey struct{}

type instanceFunc func(ctx context.Context, pri *principal.Key, instance string) (server.Status, error)

const intro = `<node><interface name="` + bus.DaemonIface + `"></interface>` + introspect.IntrospectDataString + `</node>`

var home = os.Getenv("HOME")

var c struct {
	Runtime gateruntime.Config

	Image struct {
		VarDir string
	}

	Plugin struct {
		LibDir string
	}

	Service map[string]interface{}

	Principal server.AccessConfig
}

var terminate = make(chan os.Signal, 1)

func main() {
	defer func() {
		x := recover()
		if err, ok := x.(error); ok {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		panic(x)
	}()

	os.Exit(mainResult())
}

func parseConfig(flags *flag.FlagSet, skipUnknown bool) {
	flags.Var(confi.FlagReader(c, skipUnknown), "f", "read a configuration file")
	flags.Var(confi.FlagSetter(c, skipUnknown), "o", "set a configuration option (path.to.key=value)")
	flags.Parse(os.Args[1:])
}

func mainResult() int {
	c.Runtime = gateruntime.DefaultConfig
	if home != "" {
		c.Image.VarDir = path.Join(home, ".gate", "image")
	}
	c.Plugin.LibDir = plugin.DefaultLibDir
	c.Principal = server.DefaultAccessConfig
	c.Principal.MaxModules = 1e9
	c.Principal.MaxProcs = 1e9
	c.Principal.TotalStorageSize = math.MaxInt32
	c.Principal.TotalResidentSize = math.MaxInt32
	c.Principal.MaxModuleSize = math.MaxInt32
	c.Principal.MaxTextSize = compile.MaxTextSize
	c.Principal.MaxStackSize = compile.MaxTextSize / 2
	c.Principal.MaxMemorySize = compile.MaxMemorySize
	c.Principal.TimeResolution = 1 // Best.

	flags := flag.NewFlagSet("", flag.ContinueOnError)
	flags.SetOutput(ioutil.Discard)
	parseConfig(flags, true)

	plugins, err := plugin.OpenAll(c.Plugin.LibDir)
	if err != nil {
		log.Fatal(err)
	}
	c.Service = plugins.ServiceConfig

	originConfig := origin.DefaultConfig
	originConfig.MaxConns = 1e9
	originConfig.BufSize = origin.DefaultBufSize
	c.Service[origin.ServiceName] = &originConfig

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [options]\n\nOptions:\n", flag.CommandLine.Name())
		flag.PrintDefaults()
	}
	flag.Usage = confi.FlagUsage(nil, c)
	parseConfig(flag.CommandLine, false)

	serviceRegistry := new(service.Registry)
	check(plugins.InitServices(serviceRegistry))

	c.Principal.Services = func(ctx context.Context) server.InstanceServices {
		o := origin.New(originConfig)
		r := serviceRegistry.Clone()
		r.Register(o)
		r.Register(catalog.New(r))
		return server.NewInstanceServices(o, r)
	}

	c.Principal.Debug = debugHandler

	var storage image.Storage = image.Memory
	if c.Image.VarDir != "" {
		check(os.MkdirAll(c.Image.VarDir, 0755))
		fs, err := image.NewFilesystem(c.Image.VarDir)
		check(err)
		defer fs.Close()
		storage = image.CombinedStorage(fs, image.PersistentMemory(fs))
	}

	exec, err := gateruntime.NewExecutor(c.Runtime)
	check(err)
	defer exec.Close()

	conn, err := dbus.SessionBusPrivate()
	check(err)
	defer conn.Close()
	check(conn.Auth(nil))
	check(conn.Hello())
	ctx := conn.Context()

	signal.Ignore(syscall.SIGHUP)
	signal.Notify(terminate, syscall.SIGINT, syscall.SIGTERM)

	reply, err := conn.RequestName(bus.DaemonIface, dbus.NameFlagDoNotQueue)
	check(err)
	switch reply {
	case dbus.RequestNameReplyPrimaryOwner:
		// ok
	case dbus.RequestNameReplyExists:
		return 3
	default:
		panic(fmt.Errorf("D-Bus name already taken: %s", bus.DaemonIface))
	}

	pri := new(principal.Key)

	s, err := server.New(server.Config{
		ImageStorage:   storage,
		ProcessFactory: exec,
		AccessPolicy:   &server.PublicAccess{AccessConfig: c.Principal},
		XXX_Owner:      pri,
	})
	check(err)
	defer s.Shutdown(ctx)

	check(conn.ExportMethodTable(methods(ctx, pri, s), bus.DaemonPath, bus.DaemonIface))
	check(conn.Export(introspect.Introspectable(intro), bus.DaemonPath,
		"org.freedesktop.DBus.Introspectable"))

	_, err = daemon.SdNotify(false, daemon.SdNotifyReady)
	check(err)

	<-terminate
	daemon.SdNotify(false, daemon.SdNotifyStopping)
	return 0
}

func methods(ctx context.Context, pri *principal.Key, s *server.Server) map[string]interface{} {
	methods := map[string]interface{}{
		"CallKey": func(key, function string, rFD, wFD, debugFD dbus.UnixFD, debugName string,
		) (state server.State, cause server.Cause, result int32, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			state, cause, result = handleCall(ctx, pri, s, nil, key, function, false, rFD, wFD, debugFD, debugName)
			return
		},

		"CallFile": func(moduleFD dbus.UnixFD, function string, ref bool, rFD, wFD, debugFD dbus.UnixFD, debugName string,
		) (state server.State, cause server.Cause, result int32, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			module := os.NewFile(uintptr(moduleFD), "module")
			defer module.Close()
			state, cause, result = handleCall(ctx, pri, s, module, "", function, ref, rFD, wFD, debugFD, debugName)
			return
		},

		"Download": func(moduleFD dbus.UnixFD, key string,
		) (moduleLen int64, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			module := os.NewFile(uintptr(moduleFD), "module")
			r, moduleLen := handleModuleDownload(ctx, pri, s, key)
			go func() {
				defer module.Close()
				defer r.Close()
				io.Copy(module, r)
			}()
			return
		},

		"Instances": func() (list []server.InstanceStatus, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			list = handleInstanceList(ctx, pri, s)
			return
		},

		"IO": func(instID string, rFD, wFD dbus.UnixFD) (ok bool, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			ok = handleInstanceConnect(ctx, pri, s, instID, rFD, wFD)
			return
		},

		"LaunchKey": func(key, function string, debugFD dbus.UnixFD, debugName string,
		) (instID string, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			instID = handleLaunch(ctx, pri, s, nil, key, function, false, debugFD, debugName)
			return
		},

		"LaunchFile": func(moduleFD dbus.UnixFD, function string, ref bool, debugFD dbus.UnixFD, debugName string,
		) (instID string, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			module := os.NewFile(uintptr(moduleFD), "module")
			defer module.Close()
			instID = handleLaunch(ctx, pri, s, module, "", function, ref, debugFD, debugName)
			return
		},

		"Modules": func() (list []server.ModuleRef, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			list = handleModuleList(ctx, pri, s)
			return
		},

		"Resume": func(instID string, debugFD dbus.UnixFD, debugName string,
		) (err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			handleInstanceResume(ctx, pri, s, instID, debugFD, debugName)
			return
		},

		"Upload": func(moduleFD dbus.UnixFD, moduleLen int64, key string,
		) (err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			module := os.NewFile(uintptr(moduleFD), "module")
			defer module.Close()
			handleModuleUpload(ctx, pri, s, module, moduleLen, key)
			return
		},
	}

	for name, f := range map[string]instanceFunc{
		"Delete":  s.DeleteInstance,
		"Status":  s.InstanceStatus,
		"Suspend": s.SuspendInstance,
		"Wait":    s.WaitInstance,
	} {
		f := f // Closure needs a local copy of the iterator's current value.
		methods[name] = func(instID string) (state server.State, cause server.Cause, result int32, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			state, cause, result = handleInstance(ctx, pri, f, instID)
			return
		}
	}

	return methods
}

func debugHandler(ctx context.Context, option string,
) (status string, output io.WriteCloser, err error) {
	if option != "" {
		status = option
		output = ctx.Value(debugKey{}).(*fileCell).steal()
	}
	return
}

func handleModuleList(ctx context.Context, pri *principal.Key, s *server.Server) server.ModuleRefs {
	refs, err := s.ModuleRefs(ctx, pri)
	check(err)
	sort.Sort(refs)
	return refs
}

func handleModuleDownload(ctx context.Context, pri *principal.Key, s *server.Server, key string,
) (content io.ReadCloser, contentLen int64) {
	content, contentLen, err := s.ModuleContent(ctx, pri, key)
	check(err)
	return
}

func handleModuleUpload(ctx context.Context, pri *principal.Key, s *server.Server, module *os.File, moduleLen int64, key string) {
	check(s.UploadModule(ctx, pri, true, key, module, moduleLen))
}

func handleCall(ctx context.Context, pri *principal.Key, s *server.Server, module *os.File, key, function string, ref bool, rFD, wFD, debugFD dbus.UnixFD, debugName string,
) (state server.State, cause server.Cause, result int32) {
	debug := newFileCell(debugFD, "debug")
	defer debug.Close()

	var err error
	if err == nil {
		err = syscall.SetNonblock(int(rFD), true)
	}
	if err == nil {
		err = syscall.SetNonblock(int(wFD), true)
	}
	r := os.NewFile(uintptr(rFD), "r")
	defer r.Close()
	w := os.NewFile(uintptr(wFD), "w")
	defer w.Close()
	if err != nil {
		panic(err) // First SetNonblock error.
	}

	ctx = context.WithValue(ctx, debugKey{}, debug)

	var inst *server.Instance
	if module != nil {
		moduleR, moduleLen := getReaderWithLength(module)
		inst, err = s.UploadModuleInstance(ctx, pri, ref, "", ioutil.NopCloser(moduleR), moduleLen, false, function, "", debugName)
	} else {
		inst, err = s.CreateInstance(ctx, pri, key, false, function, "", debugName)
	}
	check(err)
	defer inst.Kill(s)

	go inst.Run(ctx, s)
	inst.Connect(ctx, r, w)
	status := inst.Wait(ctx)
	return status.State, status.Cause, status.Result
}

func handleLaunch(ctx context.Context, pri *principal.Key, s *server.Server, module *os.File, key, function string, ref bool, debugFD dbus.UnixFD, debugName string) string {
	debug := newFileCell(debugFD, "debug")
	defer debug.Close()

	ctx = context.WithValue(ctx, debugKey{}, debug)

	var (
		inst *server.Instance
		err  error
	)
	if module != nil {
		moduleR, moduleLen := getReaderWithLength(module)
		inst, err = s.UploadModuleInstance(ctx, pri, ref, "", ioutil.NopCloser(moduleR), moduleLen, true, function, "", debugName)
	} else {
		inst, err = s.CreateInstance(ctx, pri, key, true, function, "", debugName)
	}
	check(err)

	go inst.Run(server.DetachedContext(ctx, pri), s)

	return inst.ID()
}

func handleInstanceList(ctx context.Context, pri *principal.Key, s *server.Server) server.Instances {
	instances, err := s.Instances(ctx, pri)
	check(err)
	sort.Sort(instances)
	return instances
}

func handleInstance(ctx context.Context, pri *principal.Key, f instanceFunc, instID string,
) (state server.State, cause server.Cause, result int32) {
	status, err := f(ctx, pri, instID)
	check(err)
	return status.State, status.Cause, status.Result
}

func handleInstanceResume(ctx context.Context, pri *principal.Key, s *server.Server, instID string, debugFD dbus.UnixFD, debugName string) {
	debug := newFileCell(debugFD, "debug")
	defer debug.Close()

	ctx = context.WithValue(ctx, debugKey{}, debug)

	inst, err := s.ResumeInstance(ctx, pri, "", instID, debugName)
	check(err)

	go inst.Run(server.DetachedContext(ctx, pri), s)
}

func handleInstanceConnect(ctx context.Context, pri *principal.Key, s *server.Server, instID string, rFD, wFD dbus.UnixFD) bool {
	var err error
	if err == nil {
		err = syscall.SetNonblock(int(rFD), true)
	}
	if err == nil {
		err = syscall.SetNonblock(int(wFD), true)
	}
	r := os.NewFile(uintptr(rFD), "r")
	defer r.Close()
	w := os.NewFile(uintptr(wFD), "w")
	defer w.Close()
	if err != nil {
		panic(err) // First SetNonblock error.
	}

	connIO, err := s.InstanceConnection(ctx, pri, instID)
	check(err)
	if connIO == nil {
		return false
	}

	_, err = connIO(ctx, r, w)
	check(err)
	return true
}

func getReaderWithLength(f *os.File) (io.Reader, int64) {
	if info, err := f.Stat(); err == nil && info.Mode().IsRegular() {
		return f, info.Size()
	}

	data, err := ioutil.ReadAll(f)
	check(err)
	return bytes.NewReader(data), int64(len(data))
}

func asBusError(x interface{}) *dbus.Error {
	if x != nil {
		if err, ok := x.(error); ok {
			if _, ok := err.(goruntime.Error); ok {
				panic(x)
			}
			return dbus.MakeFailedError(err)
		}
		panic(x)
	}
	return nil
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}
