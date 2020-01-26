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
	"math"
	"os"
	"os/signal"
	"path"
	goruntime "runtime"
	"sort"
	"strconv"
	"syscall"

	"github.com/coreos/go-systemd/v22/daemon"
	dbus "github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/tsavola/confi"
	"github.com/tsavola/gate/image"
	"github.com/tsavola/gate/internal/bus"
	inprincipal "github.com/tsavola/gate/internal/principal"
	"github.com/tsavola/gate/principal"
	gateruntime "github.com/tsavola/gate/runtime"
	"github.com/tsavola/gate/scope/program/system"
	"github.com/tsavola/gate/server"
	api "github.com/tsavola/gate/serverapi"
	"github.com/tsavola/gate/service"
	"github.com/tsavola/gate/service/catalog"
	"github.com/tsavola/gate/service/origin"
	"github.com/tsavola/gate/service/plugin"
	"github.com/tsavola/wag/compile"
)

type instanceStatusFunc func(context.Context, string) (api.Status, error)
type instanceObjectFunc func(context.Context, string) (*server.Instance, error)

const intro = `<node><interface name="` + bus.DaemonIface + `"></interface>` + introspect.IntrospectDataString + `</node>`

var userID = strconv.Itoa(os.Getuid())
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
	check(err)
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

	s, err := server.New(server.Config{
		ImageStorage:   storage,
		ProcessFactory: exec,
		AccessPolicy:   &access{server.PublicAccess{AccessConfig: c.Principal}},
		XXX_Owner:      inprincipal.LocalID,
	})
	check(err)
	defer s.Shutdown(ctx)

	ctx = principal.ContextWithID(ctx, inprincipal.LocalID)

	check(conn.ExportMethodTable(methods(ctx, s), bus.DaemonPath, bus.DaemonIface))
	check(conn.Export(introspect.Introspectable(intro), bus.DaemonPath,
		"org.freedesktop.DBus.Introspectable"))

	_, err = daemon.SdNotify(false, daemon.SdNotifyReady)
	check(err)

	<-terminate
	daemon.SdNotify(false, daemon.SdNotifyStopping)
	return 0
}

func methods(ctx context.Context, s *server.Server) map[string]interface{} {
	methods := map[string]interface{}{
		"CallKey": func(key, function string, rFD, wFD, suspendFD, debugFD dbus.UnixFD, debugLogging bool, scope []string,
		) (instID string, state api.State, cause api.Cause, result int32, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			instID, state, cause, result = handleCall(ctx, s, nil, key, function, false, rFD, wFD, suspendFD, debugFD, debugLogging, scope)
			return
		},

		"CallFile": func(moduleFD dbus.UnixFD, function string, ref bool, rFD, wFD, suspendFD, debugFD dbus.UnixFD, debugLogging bool, scope []string,
		) (instID string, state api.State, cause api.Cause, result int32, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			module := os.NewFile(uintptr(moduleFD), "module")
			defer module.Close()
			instID, state, cause, result = handleCall(ctx, s, module, "", function, ref, rFD, wFD, suspendFD, debugFD, debugLogging, scope)
			return
		},

		"Debug": func(instID string, req []byte,
		) (res []byte, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			res = handleInstanceDebug(ctx, s, instID, req)
			return
		},

		"Delete": func(instID string) (err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			handleInstanceDelete(ctx, s, instID)
			return
		},

		"Download": func(moduleFD dbus.UnixFD, key string,
		) (moduleLen int64, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			module := os.NewFile(uintptr(moduleFD), "module")
			r, moduleLen := handleModuleDownload(ctx, s, key)
			go func() {
				defer module.Close()
				defer r.Close()
				io.Copy(module, r)
			}()
			return
		},

		"Instances": func() (list api.Instances, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			list = handleInstanceList(ctx, s)
			return
		},

		"IO": func(instID string, rFD, wFD dbus.UnixFD) (ok bool, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			ok = handleInstanceConnect(ctx, s, instID, rFD, wFD)
			return
		},

		"LaunchKey": func(key, function string, suspend bool, debugFD dbus.UnixFD, debugLogging bool, scope []string,
		) (instID string, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			instID = handleLaunch(ctx, s, nil, key, function, false, suspend, debugFD, debugLogging, scope)
			return
		},

		"LaunchFile": func(moduleFD dbus.UnixFD, function string, ref, suspend bool, debugFD dbus.UnixFD, debugLogging bool, scope []string,
		) (instID string, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			module := os.NewFile(uintptr(moduleFD), "module")
			defer module.Close()
			instID = handleLaunch(ctx, s, module, "", function, ref, suspend, debugFD, debugLogging, scope)
			return
		},

		"ModuleRefs": func() (list api.ModuleRefs, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			list = handleModuleList(ctx, s)
			return
		},

		"Resume": func(instID, function string, debugFD dbus.UnixFD, debugLogging bool, scope []string,
		) (err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			handleInstanceResume(ctx, s, instID, function, debugFD, debugLogging, scope)
			return
		},

		"Snapshot": func(instID string) (progID string, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			progID = handleInstanceSnapshot(ctx, s, instID)
			return
		},

		"Unref": func(key string) (err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			handleModuleUnref(ctx, s, key)
			return
		},

		"Upload": func(moduleFD dbus.UnixFD, moduleLen int64, key string,
		) (progID string, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			module := os.NewFile(uintptr(moduleFD), "module")
			defer module.Close()
			progID = handleModuleUpload(ctx, s, module, moduleLen, key)
			return
		},
	}

	for name, f := range map[string]instanceStatusFunc{
		"Status": s.InstanceStatus,
		"Wait":   s.WaitInstance,
	} {
		f := f // Closure needs a local copy of the iterator's current value.
		methods[name] = func(instID string) (state api.State, cause api.Cause, result int32, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			state, cause, result = handleInstanceStatus(ctx, f, instID)
			return
		}
	}

	for name, f := range map[string]instanceObjectFunc{
		"Kill":    s.KillInstance,
		"Suspend": s.SuspendInstance,
	} {
		f := f // Closure needs a local copy of the iterator's current value.
		methods[name] = func(instID string) (err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			handleInstanceObject(ctx, f, instID)
			return
		}
	}

	return methods
}

func handleModuleList(ctx context.Context, s *server.Server) api.ModuleRefs {
	refs, err := s.ModuleRefs(ctx)
	check(err)
	sort.Sort(refs)
	return refs
}

func handleModuleDownload(ctx context.Context, s *server.Server, key string,
) (content io.ReadCloser, contentLen int64) {
	content, contentLen, err := s.ModuleContent(ctx, key)
	check(err)
	return
}

func handleModuleUpload(ctx context.Context, s *server.Server, module *os.File, moduleLen int64, key string) string {
	progID, err := s.UploadModule(ctx, true, key, module, moduleLen)
	check(err)
	return progID
}

func handleModuleUnref(ctx context.Context, s *server.Server, key string) {
	check(s.UnrefModule(ctx, key))
}

func handleCall(ctx context.Context, s *server.Server, module *os.File, key, function string, ref bool, rFD, wFD, suspendFD, debugFD dbus.UnixFD, debugLogging bool, scope []string,
) (instID string, state api.State, cause api.Cause, result int32) {
	debugLog := newDebugLog(debugFD, debugLogging)
	defer func() {
		if debugLog != nil {
			debugLog.Close()
		}
	}()

	var err error
	if err == nil {
		err = syscall.SetNonblock(int(rFD), true)
	}
	if err == nil {
		err = syscall.SetNonblock(int(wFD), true)
	}
	if err == nil {
		err = syscall.SetNonblock(int(suspendFD), true)
	}

	var (
		r       = os.NewFile(uintptr(rFD), "r")
		w       = os.NewFile(uintptr(wFD), "w")
		suspend = os.NewFile(uintptr(suspendFD), "suspend")
	)
	defer func() {
		r.Close()
		w.Close()
		if suspend != nil {
			suspend.Close()
		}
	}()

	if err != nil {
		panic(err) // First SetNonblock error.
	}

	ctx = server.ContextWithScope(ctx, scope)

	var inst *server.Instance
	if module != nil {
		moduleR, moduleLen := getReaderWithLength(module)
		inst, err = s.UploadModuleInstance(ctx, ioutil.NopCloser(moduleR), moduleLen, "", ref, "", function, true, false, debugLog)
	} else {
		inst, err = s.CreateInstance(ctx, key, "", function, true, false, debugLog)
	}
	debugLog = nil
	check(err)
	defer inst.Kill()

	go func(suspend *os.File) {
		defer suspend.Close()
		if n, _ := io.ReadFull(suspend, make([]byte, 1)); n > 0 {
			inst.Suspend()
		}
	}(suspend)
	suspend = nil

	inst.Connect(ctx, r, w)
	status := inst.Wait(ctx)
	return inst.ID, status.State, status.Cause, status.Result
}

func handleLaunch(ctx context.Context, s *server.Server, module *os.File, key, function string, ref, suspend bool, debugFD dbus.UnixFD, debugLogging bool, scope []string) string {
	debugLog := newDebugLog(debugFD, debugLogging)
	defer func() {
		if debugLog != nil {
			debugLog.Close()
		}
	}()

	ctx = server.ContextWithScope(ctx, scope)

	var (
		inst *server.Instance
		err  error
	)
	if module != nil {
		moduleR, moduleLen := getReaderWithLength(module)
		inst, err = s.UploadModuleInstance(ctx, ioutil.NopCloser(moduleR), moduleLen, "", ref, "", function, false, suspend, debugLog)
	} else {
		inst, err = s.CreateInstance(ctx, key, "", function, false, suspend, debugLog)
	}
	debugLog = nil
	check(err)

	return inst.ID
}

func handleInstanceList(ctx context.Context, s *server.Server) api.Instances {
	instances, err := s.Instances(ctx)
	check(err)
	sort.Sort(instances)
	return instances
}

func handleInstanceStatus(ctx context.Context, f instanceStatusFunc, instID string,
) (state api.State, cause api.Cause, result int32) {
	status, err := f(ctx, instID)
	check(err)
	return status.State, status.Cause, status.Result
}

func handleInstanceObject(ctx context.Context, f instanceObjectFunc, instID string) {
	_, err := f(ctx, instID)
	check(err)
}

func handleInstanceDelete(ctx context.Context, s *server.Server, instID string) {
	check(s.DeleteInstance(ctx, instID))
}

func handleInstanceResume(ctx context.Context, s *server.Server, instID, function string, debugFD dbus.UnixFD, debugLogging bool, scope []string) {
	debugLog := newDebugLog(debugFD, debugLogging)
	defer func() {
		if debugLog != nil {
			debugLog.Close()
		}
	}()

	ctx = server.ContextWithScope(ctx, scope)

	_, err := s.ResumeInstance(ctx, instID, function, debugLog)
	debugLog = nil
	check(err)
}

func handleInstanceConnect(ctx context.Context, s *server.Server, instID string, rFD, wFD dbus.UnixFD) bool {
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

	_, connIO, err := s.InstanceConnection(ctx, instID)
	check(err)
	if connIO == nil {
		return false
	}

	check(connIO(ctx, r, w))
	return true
}

func handleInstanceSnapshot(ctx context.Context, s *server.Server, instID string,
) (progID string) {
	progID, err := s.InstanceModule(ctx, instID)
	check(err)
	return
}

func handleInstanceDebug(ctx context.Context, s *server.Server, instID string, reqBuf []byte,
) (resBuf []byte) {
	var req api.DebugRequest
	check(req.Unmarshal(reqBuf))

	res, err := s.DebugInstance(ctx, instID, req)
	check(err)

	resBuf, err = res.Marshal()
	check(err)
	return
}

type access struct {
	server.PublicAccess
}

func (a *access) AuthorizeInstance(ctx context.Context, res *server.ResourcePolicy, inst *server.InstancePolicy,
) (context.Context, error) {
	ctx, err := a.PublicAccess.AuthorizeInstance(ctx, res, inst)
	if err != nil {
		return ctx, err
	}

	return authorizeScope(ctx)
}

func (a *access) AuthorizeProgramInstance(ctx context.Context, res *server.ResourcePolicy, prog *server.ProgramPolicy, inst *server.InstancePolicy,
) (context.Context, error) {
	ctx, err := a.PublicAccess.AuthorizeProgramInstance(ctx, res, prog, inst)
	if err != nil {
		return ctx, err
	}

	return authorizeScope(ctx)
}

func (a *access) AuthorizeProgramInstanceSource(ctx context.Context, res *server.ResourcePolicy, prog *server.ProgramPolicy, inst *server.InstancePolicy, src server.Source,
) (context.Context, error) {
	ctx, err := a.PublicAccess.AuthorizeProgramInstanceSource(ctx, res, prog, inst, src)
	if err != nil {
		return ctx, err
	}

	return authorizeScope(ctx)
}

func authorizeScope(ctx context.Context) (context.Context, error) {
	if server.ScopeContains(ctx, system.Scope) {
		ctx = system.ContextWithUserID(ctx, userID)
	}
	return ctx, nil
}

func getReaderWithLength(f *os.File) (io.Reader, int64) {
	if info, err := f.Stat(); err == nil && info.Mode().IsRegular() {
		return f, info.Size()
	}

	data, err := ioutil.ReadAll(f)
	check(err)
	return bytes.NewReader(data), int64(len(data))
}

func newDebugLog(fd dbus.UnixFD, enabled bool) io.WriteCloser {
	f := os.NewFile(uintptr(fd), "debug")
	if enabled {
		return f
	}
	f.Close()
	return nil
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
