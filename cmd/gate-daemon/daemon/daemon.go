// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package daemon

import (
	"bytes"
	"context"
	stdsql "database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"gate.computer/gate/database"
	"gate.computer/gate/database/sql"
	"gate.computer/gate/image"
	"gate.computer/gate/principal"
	"gate.computer/gate/runtime"
	"gate.computer/gate/scope"
	"gate.computer/gate/scope/program/system"
	"gate.computer/gate/server"
	"gate.computer/gate/server/api"
	"gate.computer/gate/server/webserver"
	"gate.computer/gate/service"
	"gate.computer/gate/service/origin"
	"gate.computer/gate/service/random"
	"gate.computer/internal/bus"
	"gate.computer/internal/cmdconf"
	"gate.computer/internal/logging"
	"gate.computer/internal/services"
	"gate.computer/otel/trace/tracelink"
	"gate.computer/otel/trace/tracing"
	"gate.computer/wag/compile"
	"github.com/coreos/go-systemd/v22/daemon"
	"github.com/godbus/dbus/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/proto"
	"import.name/confi"
	"import.name/pan"

	. "import.name/pan/mustcheck"
	. "import.name/type/context"
)

const (
	DefaultNewuidmap      = "newuidmap"
	DefaultNewgidmap      = "newgidmap"
	DefaultImageStateDir  = "${XDG_STATE_HOME}/gate/image"
	DefaultDatabaseDriver = "sqlite"
	DefaultInventoryDSN   = "file:${XDG_STATE_HOME}/gate/inventory/inventory.sqlite?cache=shared"
)

var DefaultConfigFiles = []string{
	"${XDG_CONFIG_HOME}/gate/daemon.toml",
	"${XDG_CONFIG_HOME}/gate/daemon.d/*.toml",
}

type Config struct {
	Runtime struct {
		runtime.Config
		PrepareProcesses int
	}

	Image struct {
		StateDir string
	}

	Inventory map[string]database.Config

	Service map[string]any

	Server struct {
		server.Config
	}

	Principal server.AccessConfig

	HTTP struct {
		Addr string
		webserver.Config

		Static []struct {
			URI  string
			Path string
		}
	}

	Log struct {
		Journal bool
	}
}

var c = new(Config)

var userID = strconv.Itoa(os.Getuid())

var terminate = make(chan os.Signal, 1)

var (
	debugLogMu sync.Mutex
	debugLogs  = make(map[string]io.WriteCloser)
)

func Main() {
	defer func() {
		pan.Fatal(recover())
	}()

	os.Exit(mainResult())
}

func mainResult() int {
	drivers := stdsql.Drivers()
	defaultDB := len(drivers) == 1 && drivers[0] == DefaultDatabaseDriver && sql.DefaultConfig == (sql.Config{})
	if defaultDB {
		sql.DefaultConfig = sql.Config{
			Driver: DefaultDatabaseDriver,
		}
	}

	c.Runtime.Config = runtime.DefaultConfig
	c.Runtime.Container.Namespace.Newuidmap = DefaultNewuidmap
	c.Runtime.Container.Namespace.Newgidmap = DefaultNewgidmap
	c.Image.StateDir = cmdconf.ExpandEnv(DefaultImageStateDir)
	c.Inventory = database.NewInventoryConfigs()
	c.Service = service.Config()
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
	flags.SetOutput(io.Discard)
	cmdconf.Parse(c, flags, true, DefaultConfigFiles...)

	if defaultDB {
		if len(c.Inventory) == 1 && Must(confi.Get(c, "inventory.sql.driver")) == DefaultDatabaseDriver && Must(confi.Get(c, "inventory.sql.dsn")) == "" {
			confi.MustSet(c, "inventory.sql.dsn", cmdconf.ExpandEnv(DefaultInventoryDSN))
		}
	}

	originConfig := origin.DefaultConfig
	originConfig.MaxConns = 1e9
	originConfig.BufSize = origin.DefaultBufSize
	c.Service["origin"] = &originConfig

	randomConfig := random.DefaultConfig
	c.Service["random"] = &randomConfig

	c.HTTP.Static = nil

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [options]\n\nOptions:\n", flag.CommandLine.Name())
		flag.PrintDefaults()
	}
	flag.Usage = confi.FlagUsage(nil, c)
	cmdconf.Parse(c, flag.CommandLine, false, DefaultConfigFiles...)

	if c.Log.Journal {
		log.SetFlags(0)
	}
	log, err := logging.Init(c.Log.Journal)
	if err != nil {
		log.Error("journal initialization failed", "error", err)
		os.Exit(1)
	}
	c.Runtime.Log = log
	c.HTTP.StartSpan = tracing.HTTPSpanStarter(nil)
	c.HTTP.AddEvent = tracing.EventAdder()
	c.HTTP.DetachTrace = tracing.TraceDetacher()

	c.Principal.Services = Must(services.Init(context.Background(), &originConfig, &randomConfig, log))

	exec := Must(runtime.NewExecutor(&c.Runtime.Config))
	defer exec.Close()

	Check(clearCaps())

	var storage image.Storage = image.Memory
	if c.Image.StateDir != "" {
		Check(os.MkdirAll(c.Image.StateDir, 0o755))
		fs := Must(image.NewFilesystem(c.Image.StateDir))
		defer fs.Close()
		storage = image.CombinedStorage(fs, image.PersistentMemory(fs))
	}

	conn := Must(dbus.SessionBusPrivate())
	defer conn.Close()
	Check(conn.Auth(nil))
	Check(conn.Hello())
	ctx := conn.Context()

	signal.Ignore(syscall.SIGHUP)
	signal.Notify(terminate, syscall.SIGINT, syscall.SIGTERM)

	inventoryDB := Must(database.Resolve(c.Inventory))
	defer inventoryDB.Close()
	inventory := Must(inventoryDB.InitInventory(ctx))

	ctx = principal.ContextWithLocalID(ctx)

	inited := make(chan api.Server, 1)
	defer close(inited)
	Check(conn.ExportMethodTable(methods(ctx, inited), bus.DaemonPath, bus.DaemonIface))

	reply := Must(conn.RequestName(bus.DaemonIface, dbus.NameFlagDoNotQueue))
	switch reply {
	case dbus.RequestNameReplyPrimaryOwner:
		// ok
	case dbus.RequestNameReplyExists:
		return 3
	default:
		panic(reply)
	}

	c.Server.ImageStorage = storage
	c.Server.Inventory = inventory
	c.Server.ProcessFactory = exec
	c.Server.AccessPolicy = &access{server.PublicAccess{AccessConfig: c.Principal}}
	c.Server.OpenDebugLog = openDebugLog
	c.Server.StartSpan = tracing.SpanStarter(nil)
	c.Server.AddEvent = tracing.EventAdder()
	if n := c.Runtime.PrepareProcesses; n > 0 {
		c.Server.ProcessFactory = runtime.PrepareProcesses(ctx, exec, n)
	}

	s := Must(server.New(ctx, &c.Server.Config))
	defer s.Shutdown(ctx)

	httpDone := make(chan error, 1)
	if c.HTTP.Addr != "" {
		host, port, err := net.SplitHostPort(c.HTTP.Addr)
		Check(err)
		if host == "" {
			Check(errors.New("HTTP hostname must be configured explicitly"))
		}
		verifyLoopbackHost("HTTP", host)

		if c.HTTP.Authority == "" {
			if port == "80" || port == "http" {
				c.HTTP.Authority = host
			} else {
				c.HTTP.Authority = c.HTTP.Addr
			}
		}

		if len(c.HTTP.Origins) == 0 {
			Check(errors.New("no HTTP origins configured"))
		}
		for _, origin := range c.HTTP.Origins {
			if origin != "" && origin != "null" {
				u := Must(url.Parse(origin))
				verifyLoopbackHost("HTTP origin", u.Hostname())
			}
		}

		c.HTTP.Server = s
		apiHandler := webserver.NewHandlerWithUnsecuredLocalAuthorization("/", &c.HTTP.Config)
		handler := newHTTPHandler(apiHandler, "http://"+c.HTTP.Authority)

		go func() {
			defer close(httpDone)
			httpDone <- http.ListenAndServe(c.HTTP.Addr, handler)
		}()
	}

	inited <- s

	Must(daemon.SdNotify(false, daemon.SdNotifyReady))

	select {
	case <-terminate:
	case err := <-httpDone:
		Check(err)
	}

	daemon.SdNotify(false, daemon.SdNotifyStopping)
	return 0
}

func verifyLoopbackHost(errorDesc, host string) {
	for _, ip := range Must(net.LookupIP(host)) {
		if !ip.IsLoopback() {
			Check(fmt.Errorf("%s hostname %q resolves to non-loopback IP address: %s", errorDesc, host, ip))
		}
	}
}

func methods(ctx Context, inited <-chan api.Server) map[string]any {
	var initedServer api.Server
	s := func() api.Server {
		if initedServer != nil {
			return initedServer
		}
		if inited != nil {
			initedServer = <-inited
			inited = nil
		}
		if initedServer != nil {
			return initedServer
		}
		panic(pan.Wrap(errors.New("daemon initialization was aborted")))
	}

	methods := map[string]any{
		"Call": func(traceID, spanID []byte, moduleID, function string, instanceTags, scop []string, transient bool, suspendFD, rFD, wFD, debugFD dbus.UnixFD, debugLogging bool) (instanceID string, state api.State, cause api.Cause, result int32, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			ctx, span := startSpan(ctx, "Call", traceID, spanID)
			defer span.End()
			launch := &api.LaunchOptions{
				Function:  function,
				Transient: transient,
				Tags:      instanceTags,
			}
			ctx = scope.Context(ctx, scop)
			instanceID, state, cause, result = doCall(ctx, s(), moduleID, nil, nil, launch, suspendFD, rFD, wFD, debugFD, debugLogging)
			return
		},

		"CallFile": func(traceID, spanID []byte, moduleFD dbus.UnixFD, modulePin bool, moduleTags []string, function string, instanceTags, scop []string, transient bool, suspendFD, rFD, wFD, debugFD dbus.UnixFD, debugLogging bool) (instanceID string, state api.State, cause api.Cause, result int32, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			ctx, span := startSpan(ctx, "CallFile", traceID, spanID)
			defer span.End()
			moduleFile := os.NewFile(uintptr(moduleFD), "module")
			moduleOpt := moduleOptions(modulePin, moduleTags)
			launch := &api.LaunchOptions{
				Function:  function,
				Transient: transient,
				Tags:      instanceTags,
			}
			ctx = scope.Context(ctx, scop)
			instanceID, state, cause, result = doCall(ctx, s(), "", moduleFile, moduleOpt, launch, suspendFD, rFD, wFD, debugFD, debugLogging)
			return
		},

		"ConnectInstance": func(traceID, spanID []byte, instanceID string, rFD, wFD dbus.UnixFD) (ok bool, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			ctx, span := startSpan(ctx, "ConnectInstance", traceID, spanID)
			defer span.End()
			ok = connectInstance(ctx, s(), instanceID, rFD, wFD)
			return
		},

		"DebugInstance": func(traceID, spanID []byte, instanceID string, reqProtoBuf []byte) (resProtoBuf []byte, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			ctx, span := startSpan(ctx, "DebugInstance", traceID, spanID)
			defer span.End()
			resProtoBuf = debugInstance(ctx, s(), instanceID, reqProtoBuf)
			return
		},

		"DeleteInstance": func(traceID, spanID []byte, instanceID string) (err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			ctx, span := startSpan(ctx, "DeleteInstance", traceID, spanID)
			defer span.End()
			Check(s().DeleteInstance(ctx, instanceID))
			return
		},

		"DownloadModule": func(traceID, spanID []byte, moduleFD dbus.UnixFD, moduleID string) (moduleLen int64, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			ctx, span := startSpan(ctx, "DownloadModule", traceID, spanID)
			defer span.End()
			module := os.NewFile(uintptr(moduleFD), "module")
			r, moduleLen := downloadModule(ctx, s(), moduleID)
			go func() {
				defer module.Close()
				defer r.Close()
				io.Copy(module, r)
			}()
			return
		},

		"GetInstanceInfo": func(traceID, spanID []byte, instanceID string) (state api.State, cause api.Cause, result int32, tags []string, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			ctx, span := startSpan(ctx, "GetInstanceInfo", traceID, spanID)
			defer span.End()
			state, cause, result, tags = getInstanceInfo(ctx, s(), instanceID)
			return
		},

		"GetModuleInfo": func(traceID, spanID []byte, moduleID string) (tags []string, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			ctx, span := startSpan(ctx, "GetModuleInfo", traceID, spanID)
			defer span.End()
			tags = Must(s().ModuleInfo(ctx, moduleID)).Tags
			return
		},

		"GetScope": func(traceID, spanID []byte) (scop []string, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			_, span := startSpan(ctx, "GetScope", traceID, spanID)
			defer span.End()
			scop = s().Features().Scope
			return
		},

		"Launch": func(traceID, spanID []byte, moduleID string, function string, suspend bool, instanceTags []string, scop []string, debugFD dbus.UnixFD, debugLogging bool) (instanceID string, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			ctx, span := startSpan(ctx, "Launch", traceID, spanID)
			defer span.End()
			launch := &api.LaunchOptions{
				Function: function,
				Suspend:  suspend,
				Tags:     instanceTags,
			}
			ctx = scope.Context(ctx, scop)
			inst := doLaunch(ctx, s(), moduleID, nil, nil, launch, debugFD, debugLogging)
			instanceID = inst.ID()
			return
		},

		"LaunchFile": func(traceID, spanID []byte, moduleFD dbus.UnixFD, modulePin bool, moduleTags []string, function string, suspend bool, instanceTags, scop []string, debugFD dbus.UnixFD, debugLogging bool) (instanceID string, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			ctx, span := startSpan(ctx, "LaunchFile", traceID, spanID)
			defer span.End()
			moduleFile := os.NewFile(uintptr(moduleFD), "module")
			moduleOpt := moduleOptions(modulePin, moduleTags)
			launch := &api.LaunchOptions{
				Function: function,
				Suspend:  suspend,
				Tags:     instanceTags,
			}
			ctx = scope.Context(ctx, scop)
			inst := doLaunch(ctx, s(), "", moduleFile, moduleOpt, launch, debugFD, debugLogging)
			instanceID = inst.ID()
			return
		},

		"ListInstances": func(traceID, spanID []byte) (list []string, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			ctx, span := startSpan(ctx, "ListInstances", traceID, spanID)
			defer span.End()
			list = listInstances(ctx, s())
			return
		},

		"ListModules": func(traceID, spanID []byte) (list []string, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			ctx, span := startSpan(ctx, "ListModules", traceID, spanID)
			defer span.End()
			list = listModules(ctx, s())
			return
		},

		"PinModule": func(traceID, spanID []byte, moduleID string, tags []string) (err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			ctx, span := startSpan(ctx, "PinModule", traceID, spanID)
			defer span.End()
			opt := &api.ModuleOptions{
				Pin:  true,
				Tags: tags,
			}
			Check(s().PinModule(ctx, moduleID, opt))
			return
		},

		"KillInstance": func(traceID, spanID []byte, instanceID string) (err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			ctx, span := startSpan(ctx, "KillInstance", traceID, spanID)
			defer span.End()
			Must(s().KillInstance(ctx, instanceID))
			return
		},

		"ResumeInstance": func(traceID, spanID []byte, instanceID, function string, scop []string, debugFD dbus.UnixFD, debugLogging bool) (err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			ctx, span := startSpan(ctx, "ResumeInstance", traceID, spanID)
			defer span.End()
			resume := &api.ResumeOptions{
				Function: function,
			}
			ctx = scope.Context(ctx, scop)
			resumeInstance(ctx, s(), instanceID, resume, debugFD, debugLogging)
			return
		},

		"Snapshot": func(traceID, spanID []byte, instanceID string, moduleTags []string) (moduleID string, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			ctx, span := startSpan(ctx, "Snapshot", traceID, spanID)
			defer span.End()
			moduleOpt := moduleOptions(true, moduleTags)
			moduleID = Must(s().Snapshot(ctx, instanceID, moduleOpt))
			return
		},

		"SuspendInstance": func(traceID, spanID []byte, instanceID string) (err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			ctx, span := startSpan(ctx, "SuspendInstance", traceID, spanID)
			defer span.End()
			Must(s().SuspendInstance(ctx, instanceID))
			return
		},

		"UnpinModule": func(traceID, spanID []byte, moduleID string) (err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			ctx, span := startSpan(ctx, "UnpinModule", traceID, spanID)
			defer span.End()
			Check(s().UnpinModule(ctx, moduleID))
			return
		},

		"UpdateInstance": func(traceID, spanID []byte, instanceID string, persist bool, tags []string) (err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			ctx, span := startSpan(ctx, "UpdateInstance", traceID, spanID)
			defer span.End()
			update := &api.InstanceUpdate{
				Persist: persist,
				Tags:    tags,
			}
			Must(s().UpdateInstance(ctx, instanceID, update))
			return
		},

		"UploadModule": func(traceID, spanID []byte, fd dbus.UnixFD, length int64, hash string, tags []string) (moduleID string, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			ctx, span := startSpan(ctx, "UploadModule", traceID, spanID)
			defer span.End()
			file := os.NewFile(uintptr(fd), "module")
			opt := moduleOptions(true, tags)
			moduleID = uploadModule(ctx, s(), file, length, hash, opt)
			return
		},

		"WaitInstance": func(traceID, spanID []byte, instanceID string) (state api.State, cause api.Cause, result int32, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			ctx, span := startSpan(ctx, "WaitInstance", traceID, spanID)
			defer span.End()
			state, cause, result = waitInstance(ctx, s(), instanceID)
			return
		},
	}

	return methods
}

func listModules(ctx Context, s api.Server) []string {
	refs := Must(s.Modules(ctx))
	ids := make([]string, 0, len(refs.Modules))
	for _, ref := range refs.Modules {
		ids = append(ids, ref.Module)
	}
	return ids
}

func downloadModule(ctx Context, s api.Server, moduleID string) (io.ReadCloser, int64) {
	stream, length, err := s.ModuleContent(ctx, moduleID)
	Check(err)
	return stream, length
}

func uploadModule(ctx Context, s api.Server, file *os.File, length int64, hash string, opt *api.ModuleOptions) string {
	upload := &api.ModuleUpload{
		Stream: file,
		Length: length,
		Hash:   hash,
	}
	defer upload.Close()

	return Must(s.UploadModule(ctx, upload, opt))
}

// doCall module id or file.  Module options apply only to module file.
func doCall(ctx Context, s api.Server, moduleID string, moduleFile *os.File, moduleOpt *api.ModuleOptions, launch *api.LaunchOptions, suspendFD dbus.UnixFD, rFD dbus.UnixFD, wFD dbus.UnixFD, debugFD dbus.UnixFD, debugLogging bool) (string, api.State, api.Cause, int32) {
	syscall.SetNonblock(int(suspendFD), true)
	suspend := os.NewFile(uintptr(suspendFD), "suspend")
	defer func() {
		if suspend != nil {
			suspend.Close()
		}
	}()

	syscall.SetNonblock(int(rFD), true)
	r := os.NewFile(uintptr(rFD), "r")
	defer r.Close()

	wrote := false
	syscall.SetNonblock(int(wFD), true)
	w := os.NewFile(uintptr(wFD), "w")
	defer func() {
		if !wrote {
			w.Close()
		}
	}()

	inst := doLaunch(ctx, s, moduleID, moduleFile, moduleOpt, launch, debugFD, debugLogging)
	defer func() {
		if err := inst.Kill(ctx); err != nil {
			panic(err)
		}
	}()

	go func(suspend *os.File) {
		defer suspend.Close()
		if n, _ := suspend.Read(make([]byte, 1)); n > 0 {
			if err := inst.Suspend(ctx); err != nil {
				panic(err)
			}
		}
	}(suspend)
	suspend = nil

	wrote = true
	Check(inst.Connect(ctx, r, w))
	status := inst.Wait(ctx)
	return inst.ID(), status.State, status.Cause, status.Result
}

// doLaunch module id or file.  Module options apply only to module file.
func doLaunch(ctx Context, s api.Server, moduleID string, moduleFile *os.File, moduleOpt *api.ModuleOptions, launch *api.LaunchOptions, debugFD dbus.UnixFD, debugLogging bool) api.Instance {
	invoke, cancel := invokeOptions(debugFD, debugLogging)
	defer cancel()

	launch.Invoke = invoke

	if moduleFile != nil {
		upload := moduleUpload(moduleFile)
		defer upload.Close()

		_, inst, err := s.UploadModuleInstance(ctx, upload, moduleOpt, launch)
		Check(err)
		return inst
	} else {
		return Must(s.NewInstance(ctx, moduleID, launch))
	}
}

func listInstances(ctx Context, s api.Server) []string {
	instances := Must(s.Instances(ctx))
	ids := make([]string, 0, len(instances.Instances))
	for _, i := range instances.Instances {
		ids = append(ids, i.Instance)
	}
	return ids
}

func getInstanceInfo(ctx Context, s api.Server, instanceID string) (state api.State, cause api.Cause, result int32, tags []string) {
	info := Must(s.InstanceInfo(ctx, instanceID))
	return info.Status.State, info.Status.Cause, info.Status.Result, info.Tags
}

func waitInstance(ctx Context, s api.Server, instanceID string) (state api.State, cause api.Cause, result int32) {
	status := Must(s.WaitInstance(ctx, instanceID))
	return status.State, status.Cause, status.Result
}

func resumeInstance(ctx Context, s api.Server, instance string, resume *api.ResumeOptions, debugFD dbus.UnixFD, debugLogging bool) {
	invoke, cancel := invokeOptions(debugFD, debugLogging)
	defer cancel()

	resume.Invoke = invoke

	Must(s.ResumeInstance(ctx, instance, resume))
}

func connectInstance(ctx Context, s api.Server, instanceID string, rFD, wFD dbus.UnixFD) bool {
	var err error
	if err == nil {
		err = syscall.SetNonblock(int(rFD), true)
	}
	if err == nil {
		err = syscall.SetNonblock(int(wFD), true)
	}

	r := os.NewFile(uintptr(rFD), "r")
	defer r.Close()

	wrote := false
	w := os.NewFile(uintptr(wFD), "w")
	defer func() {
		if !wrote {
			w.Close()
		}
	}()

	Check(err) // First SetNonblock error.

	_, iofunc, err := s.InstanceConnection(ctx, instanceID)
	Check(err)
	if iofunc == nil {
		return false
	}

	link := trace.Link{SpanContext: trace.SpanContextFromContext(ctx)}
	ctx = trace.ContextWithSpanContext(ctx, trace.SpanContext{})
	ctx = tracelink.ContextWithLinks(ctx, link)

	wrote = true
	iofunc(ctx, r, w)
	return true
}

func debugInstance(ctx Context, s api.Server, instanceID string, reqBuf []byte) []byte {
	req := new(api.DebugRequest)
	Check(proto.Unmarshal(reqBuf, req))

	res := Must(s.DebugInstance(ctx, instanceID, req))

	return Must(proto.Marshal(res))
}

type access struct {
	server.PublicAccess
}

func (a *access) AuthorizeInstance(ctx Context, res *server.ResourcePolicy, inst *server.InstancePolicy) (Context, error) {
	ctx, err := a.PublicAccess.AuthorizeInstance(ctx, res, inst)
	if err != nil {
		return ctx, err
	}

	return authorizeScope(ctx)
}

func (a *access) AuthorizeProgramInstance(ctx Context, res *server.ResourcePolicy, prog *server.ProgramPolicy, inst *server.InstancePolicy) (Context, error) {
	ctx, err := a.PublicAccess.AuthorizeProgramInstance(ctx, res, prog, inst)
	if err != nil {
		return ctx, err
	}

	return authorizeScope(ctx)
}

func (a *access) AuthorizeProgramInstanceSource(ctx Context, res *server.ResourcePolicy, prog *server.ProgramPolicy, inst *server.InstancePolicy, src string) (Context, error) {
	ctx, err := a.PublicAccess.AuthorizeProgramInstanceSource(ctx, res, prog, inst, src)
	if err != nil {
		return ctx, err
	}

	return authorizeScope(ctx)
}

func authorizeScope(ctx Context) (Context, error) {
	if scope.ContextContains(ctx, system.Scope) {
		ctx = system.ContextWithUserID(ctx, userID)
	}
	return ctx, nil
}

func moduleUpload(f *os.File) *api.ModuleUpload {
	if info, err := f.Stat(); err == nil && info.Mode().IsRegular() {
		return &api.ModuleUpload{
			Stream: f,
			Length: info.Size(),
		}
	}

	data := Must(io.ReadAll(f))

	return &api.ModuleUpload{
		Stream: io.NopCloser(bytes.NewReader(data)),
		Length: int64(len(data)),
	}
}

func moduleOptions(pin bool, tags []string) *api.ModuleOptions {
	return &api.ModuleOptions{
		Pin:  pin,
		Tags: tags,
	}
}

func invokeOptions(debugFD dbus.UnixFD, debugLogging bool) (*api.InvokeOptions, func()) {
	f := os.NewFile(uintptr(debugFD), "debug")
	if !debugLogging {
		f.Close()
		return nil, func() {}
	}

	id := fmt.Sprint(debugFD)
	opt := &api.InvokeOptions{
		DebugLog: id,
	}

	cancel := func() {
		debugLogMu.Lock()
		defer debugLogMu.Unlock()

		if _, found := debugLogs[id]; found {
			delete(debugLogs, id)
			f.Close()
		}
	}

	debugLogMu.Lock()
	defer debugLogMu.Unlock()

	debugLogs[id] = f

	return opt, cancel
}

func openDebugLog(id string) io.WriteCloser {
	debugLogMu.Lock()
	defer debugLogMu.Unlock()

	f := debugLogs[id]
	delete(debugLogs, id)

	return f
}

func asBusError(x any) *dbus.Error {
	if x == nil {
		return nil
	}
	return dbus.MakeFailedError(pan.Error(x))
}

func newHTTPHandler(api http.Handler, origin string) http.Handler {
	mux := http.NewServeMux()

	for _, static := range c.HTTP.Static {
		if !strings.HasPrefix(static.URI, "/") {
			Check(fmt.Errorf("static HTTP URI does not start with slash: %q", static.URI))
		}
		if static.Path == "" {
			Check(fmt.Errorf("filesystem path not specified for static HTTP URI: %q", static.URI))
		}
		if strings.HasSuffix(static.URI, "/") != strings.HasSuffix(static.Path, "/") {
			Check(errors.New("static HTTP URI and filesystem path must both end in slash if one ends in slash"))
		}

		mux.HandleFunc(static.URI, newStaticHTTPHandler(static.URI, static.Path, origin))
	}

	mux.Handle("/", api)
	return mux
}

func newStaticHTTPHandler(staticPattern, staticPath, staticOrigin string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Server", "gate-daemon")

		switch origin := r.Header.Get("Origin"); origin {
		case "":
		case staticOrigin:
			w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD")
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Max-Age", "3600")
		default:
			w.WriteHeader(http.StatusForbidden)
			return
		}

		switch r.Method {
		case "GET", "HEAD":
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var staticFile string

		if strings.HasSuffix(staticPattern, "/") {
			if !strings.HasPrefix(r.URL.Path, staticPattern) {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			staticFile = staticPath + r.URL.Path[len(staticPattern):]
			if strings.HasSuffix(staticFile, "/") {
				staticFile += "index.html"
			}
		} else {
			if r.URL.Path != staticPattern {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			staticFile = staticPath
		}

		http.ServeFile(w, r, staticFile)
	}
}

func startSpan(ctx Context, method string, traceID, parentID []byte) (Context, trace.Span) {
	traceIDx := trace.TraceID(traceID)
	parentIDx := trace.SpanID(parentID)

	var c trace.SpanContext
	c = c.WithTraceID(traceIDx)
	c = c.WithSpanID(parentIDx)
	ctx = trace.ContextWithRemoteSpanContext(ctx, c)

	tracer := otel.GetTracerProvider().Tracer("gate/cmd/gate-daemon")
	ctx, span := tracer.Start(ctx, method, trace.WithSpanKind(trace.SpanKindServer))

	slog.DebugContext(ctx, "daemon: call",
		"method", method,
		"trace", traceIDx,
		"span", span.SpanContext().SpanID(),
		"parent", parentIDx,
	)

	return ctx, span
}
