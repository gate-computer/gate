// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package daemon

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"gate.computer/gate/image"
	"gate.computer/gate/internal/bus"
	"gate.computer/gate/internal/cmdconf"
	"gate.computer/gate/internal/defaultlog"
	"gate.computer/gate/internal/services"
	"gate.computer/gate/internal/sys"
	"gate.computer/gate/principal"
	gateruntime "gate.computer/gate/runtime"
	gatescope "gate.computer/gate/scope"
	"gate.computer/gate/scope/program/system"
	"gate.computer/gate/server"
	"gate.computer/gate/server/api"
	"gate.computer/gate/server/web"
	"gate.computer/gate/service"
	grpc "gate.computer/gate/service/grpc/config"
	"gate.computer/gate/service/origin"
	"gate.computer/wag/compile"
	"github.com/coreos/go-systemd/v22/daemon"
	dbus "github.com/godbus/dbus/v5"
	"google.golang.org/protobuf/proto"
	"import.name/confi"
	"import.name/pan"
)

const (
	DefaultNewuidmap   = "newuidmap"
	DefaultNewgidmap   = "newgidmap"
	DefaultImageVarDir = ".gate/image" // Relative to home directory.
)

// Defaults are relative to home directory.
var Defaults = []string{
	".config/gate/daemon.toml",
	".config/gate/daemon.d/*.toml",
}

type Config struct {
	Runtime struct {
		gateruntime.Config
		PrepareProcesses int
	}

	Image struct {
		VarDir string
	}

	Service map[string]interface{}

	Principal server.AccessConfig

	HTTP struct {
		Addr string
		web.Config

		Static []struct {
			URI  string
			Path string
		}
	}
}

var c = new(Config)

type instanceFunc func(*server.Server, context.Context, string) (*server.Instance, error)

var userID = strconv.Itoa(os.Getuid())

var terminate = make(chan os.Signal, 1)

var (
	debugLogMu sync.Mutex
	debugLogs  = make(map[string]io.WriteCloser)
)

func Main() {
	log.SetFlags(0)

	defer func() {
		pan.Fatal(recover())
	}()

	os.Exit(mainResult())
}

func mainResult() int {
	c.Runtime.Config = gateruntime.DefaultConfig
	c.Runtime.Container.Namespace.Newuidmap = DefaultNewuidmap
	c.Runtime.Container.Namespace.Newgidmap = DefaultNewgidmap
	c.Image.VarDir = cmdconf.JoinHomeFallback(DefaultImageVarDir, "")
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
	flags.SetOutput(ioutil.Discard)
	cmdconf.Parse(c, flags, true, Defaults...)

	c.Service["grpc"] = grpc.Config

	originConfig := origin.DefaultConfig
	originConfig.MaxConns = 1e9
	originConfig.BufSize = origin.DefaultBufSize
	c.Service["origin"] = &originConfig

	c.HTTP.Static = nil

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [options]\n\nOptions:\n", flag.CommandLine.Name())
		flag.PrintDefaults()
	}
	flag.Usage = confi.FlagUsage(nil, c)
	cmdconf.Parse(c, flag.CommandLine, false, Defaults...)

	var err error
	c.Principal.Services, err = services.Init(context.Background(), &originConfig, defaultlog.StandardLogger{})
	check(err)

	exec, err := gateruntime.NewExecutor(&c.Runtime.Config)
	check(err)
	defer exec.Close()

	check(sys.ClearCaps())

	var storage image.Storage = image.Memory
	if c.Image.VarDir != "" {
		check(os.MkdirAll(c.Image.VarDir, 0755))
		fs, err := image.NewFilesystem(c.Image.VarDir)
		check(err)
		defer fs.Close()
		storage = image.CombinedStorage(fs, image.PersistentMemory(fs))
	}

	conn, err := dbus.SessionBusPrivate()
	check(err)
	defer conn.Close()
	check(conn.Auth(nil))
	check(conn.Hello())
	ctx := conn.Context()

	signal.Ignore(syscall.SIGHUP)
	signal.Notify(terminate, syscall.SIGINT, syscall.SIGTERM)

	ctx = principal.ContextWithLocalID(ctx)

	inited := make(chan *server.Server, 1)
	defer close(inited)
	check(conn.ExportMethodTable(methods(ctx, inited), bus.DaemonPath, bus.DaemonIface))

	reply, err := conn.RequestName(bus.DaemonIface, dbus.NameFlagDoNotQueue)
	check(err)
	switch reply {
	case dbus.RequestNameReplyPrimaryOwner:
		// ok
	case dbus.RequestNameReplyExists:
		return 3
	default:
		panic(reply)
	}

	serverConfig := &server.Config{
		ImageStorage:   storage,
		ProcessFactory: exec,
		AccessPolicy:   &access{server.PublicAccess{AccessConfig: c.Principal}},
		OpenDebugLog:   openDebugLog,
	}
	if n := c.Runtime.PrepareProcesses; n > 0 {
		serverConfig.ProcessFactory = gateruntime.PrepareProcesses(ctx, exec, n)
	}

	s, err := server.New(ctx, serverConfig)
	check(err)
	defer s.Shutdown(ctx)

	httpDone := make(chan error, 1)
	if c.HTTP.Addr != "" {
		host, port, err := net.SplitHostPort(c.HTTP.Addr)
		check(err)
		if host == "" {
			check(errors.New("HTTP hostname must be configured explicitly"))
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
			check(errors.New("no HTTP origins configured"))
		}
		for _, origin := range c.HTTP.Origins {
			if origin != "" && origin != "null" {
				u, err := url.Parse(origin)
				check(err)
				verifyLoopbackHost("HTTP origin", u.Hostname())
			}
		}

		c.HTTP.Server = s
		apiHandler := web.NewHandlerWithUnsecuredLocalAuthorization("/", &c.HTTP.Config)
		handler := newHTTPHandler(apiHandler, "http://"+c.HTTP.Authority)

		go func() {
			defer close(httpDone)
			httpDone <- http.ListenAndServe(c.HTTP.Addr, handler)
		}()
	}

	inited <- s

	_, err = daemon.SdNotify(false, daemon.SdNotifyReady)
	check(err)

	select {
	case <-terminate:
	case err := <-httpDone:
		check(err)
	}

	daemon.SdNotify(false, daemon.SdNotifyStopping)
	return 0
}

func verifyLoopbackHost(errorDesc, host string) {
	ips, err := net.LookupIP(host)
	check(err)

	for _, ip := range ips {
		if !ip.IsLoopback() {
			check(fmt.Errorf("%s hostname %q resolves to non-loopback IP address: %s", errorDesc, host, ip))
		}
	}
}

func methods(ctx context.Context, inited <-chan *server.Server) map[string]interface{} {
	var initedServer *server.Server
	s := func() *server.Server {
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

	methods := map[string]interface{}{
		"Call": func(
			moduleID string,
			function string,
			instanceTags []string,
			scope []string,
			suspendFD dbus.UnixFD,
			rFD dbus.UnixFD,
			wFD dbus.UnixFD,
			debugFD dbus.UnixFD,
			debugLogging bool,
		) (instanceID string, state api.State, cause api.Cause, result int32, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			launch := &api.LaunchOptions{
				Function:  function,
				Transient: true,
				Tags:      instanceTags,
			}
			ctx = gatescope.Context(ctx, scope)
			instanceID, state, cause, result = doCall(ctx, s(), moduleID, nil, nil, launch, suspendFD, rFD, wFD, debugFD, debugLogging)
			return
		},

		"CallFile": func(
			moduleFD dbus.UnixFD,
			modulePin bool,
			moduleTags []string,
			function string,
			instanceTags []string,
			scope []string,
			suspendFD dbus.UnixFD,
			rFD dbus.UnixFD,
			wFD dbus.UnixFD,
			debugFD dbus.UnixFD,
			debugLogging bool,
		) (instanceID string, state api.State, cause api.Cause, result int32, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			moduleFile := os.NewFile(uintptr(moduleFD), "module")
			moduleOpt := moduleOptions(modulePin, moduleTags)
			launch := &api.LaunchOptions{
				Function:  function,
				Transient: true,
				Tags:      instanceTags,
			}
			ctx = gatescope.Context(ctx, scope)
			instanceID, state, cause, result = doCall(ctx, s(), "", moduleFile, moduleOpt, launch, suspendFD, rFD, wFD, debugFD, debugLogging)
			return
		},

		"ConnectInstance": func(instanceID string, rFD, wFD dbus.UnixFD) (ok bool, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			ok = connectInstance(ctx, s(), instanceID, rFD, wFD)
			return
		},

		"DebugInstance": func(instanceID string, reqProtoBuf []byte) (resProtoBuf []byte, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			resProtoBuf = debugInstance(ctx, s(), instanceID, reqProtoBuf)
			return
		},

		"DeleteInstance": func(instanceID string) (err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			deleteInstance(ctx, s(), instanceID)
			return
		},

		"DownloadModule": func(moduleFD dbus.UnixFD, moduleID string) (moduleLen int64, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			module := os.NewFile(uintptr(moduleFD), "module")
			r, moduleLen := downloadModule(ctx, s(), moduleID)
			go func() {
				defer module.Close()
				defer r.Close()
				io.Copy(module, r)
			}()
			return
		},

		"GetInstanceInfo": func(instanceID string) (state api.State, cause api.Cause, result int32, tags []string, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			state, cause, result, tags = getInstanceInfo(ctx, s(), instanceID)
			return
		},

		"GetModuleInfo": func(moduleID string) (tags []string, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			tags = getModuleInfo(ctx, s(), moduleID)
			return
		},

		"GetScope": func() (scope []string, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			scope = getScope(ctx, s())
			return
		},

		"Launch": func(
			moduleID string,
			function string,
			suspend bool,
			instanceTags []string,
			scope []string,
			debugFD dbus.UnixFD,
			debugLogging bool,
		) (instanceID string, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			launch := &api.LaunchOptions{
				Function: function,
				Suspend:  suspend,
				Tags:     instanceTags,
			}
			ctx = gatescope.Context(ctx, scope)
			inst := doLaunch(ctx, s(), moduleID, nil, nil, launch, debugFD, debugLogging)
			instanceID = inst.ID()
			return
		},

		"LaunchFile": func(
			moduleFD dbus.UnixFD,
			modulePin bool,
			moduleTags []string,
			function string,
			suspend bool,
			instanceTags []string,
			scope []string,
			debugFD dbus.UnixFD,
			debugLogging bool,
		) (instanceID string, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			moduleFile := os.NewFile(uintptr(moduleFD), "module")
			moduleOpt := moduleOptions(modulePin, moduleTags)
			launch := &api.LaunchOptions{
				Function: function,
				Suspend:  suspend,
				Tags:     instanceTags,
			}
			ctx = gatescope.Context(ctx, scope)
			inst := doLaunch(ctx, s(), "", moduleFile, moduleOpt, launch, debugFD, debugLogging)
			instanceID = inst.ID()
			return
		},

		"ListInstances": func() (list []string, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			list = listInstances(ctx, s())
			return
		},

		"ListModules": func() (list []string, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			list = listModules(ctx, s())
			return
		},

		"PinModule": func(moduleID string, tags []string) (err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			opt := &api.ModuleOptions{
				Pin:  true,
				Tags: tags,
			}
			pinModule(ctx, s(), moduleID, opt)
			return
		},

		"KillInstance": func(instanceID string) (err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			killInstance(ctx, s(), instanceID)
			return
		},

		"ResumeInstance": func(instanceID, function string, scope []string, debugFD dbus.UnixFD, debugLogging bool) (err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			resume := &api.ResumeOptions{
				Function: function,
			}
			ctx = gatescope.Context(ctx, scope)
			resumeInstance(ctx, s(), instanceID, resume, debugFD, debugLogging)
			return
		},

		"Snapshot": func(instanceID string, moduleTags []string) (moduleID string, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			moduleOpt := moduleOptions(true, moduleTags)
			moduleID = snapshot(ctx, s(), instanceID, moduleOpt)
			return
		},

		"SuspendInstance": func(instanceID string) (err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			suspendInstance(ctx, s(), instanceID)
			return
		},

		"UnpinModule": func(moduleID string) (err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			unpinModule(ctx, s(), moduleID)
			return
		},

		"UpdateInstance": func(instanceID string, persist bool, tags []string) (err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			update := &api.InstanceUpdate{
				Persist: persist,
				Tags:    tags,
			}
			updateInstance(ctx, s(), instanceID, update)
			return
		},

		"UploadModule": func(fd dbus.UnixFD, length int64, hash string, tags []string) (moduleID string, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			file := os.NewFile(uintptr(fd), "module")
			opt := moduleOptions(true, tags)
			moduleID = uploadModule(ctx, s(), file, length, hash, opt)
			return
		},

		"WaitInstance": func(instanceID string) (state api.State, cause api.Cause, result int32, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			state, cause, result = waitInstance(ctx, s(), instanceID)
			return
		},
	}

	return methods
}

func getScope(ctx context.Context, s *server.Server) []string {
	f, err := s.Features(ctx)
	check(err)
	return f.Scope
}

func listModules(ctx context.Context, s *server.Server) []string {
	refs, err := s.Modules(ctx)
	check(err)
	sort.Sort(refs)
	ids := make([]string, 0, len(refs.Modules))
	for _, ref := range refs.Modules {
		ids = append(ids, ref.Id)
	}
	return ids
}

func getModuleInfo(ctx context.Context, s *server.Server, moduleID string) (tags []string) {
	info, err := s.ModuleInfo(ctx, moduleID)
	check(err)
	return info.Tags
}

func downloadModule(ctx context.Context, s *server.Server, moduleID string) (io.ReadCloser, int64) {
	stream, length, err := s.ModuleContent(ctx, moduleID)
	check(err)
	return stream, length
}

func uploadModule(ctx context.Context, s *server.Server, file *os.File, length int64, hash string, opt *api.ModuleOptions) string {
	upload := &api.ModuleUpload{
		Stream: file,
		Length: length,
		Hash:   hash,
	}
	defer upload.Close()

	id, err := s.UploadModule(ctx, upload, opt)
	check(err)
	return id
}

func pinModule(ctx context.Context, s *server.Server, moduleID string, opt *api.ModuleOptions) {
	check(s.PinModule(ctx, moduleID, opt))
}

func unpinModule(ctx context.Context, s *server.Server, moduleID string) {
	check(s.UnpinModule(ctx, moduleID))
}

// doCall module id or file.  Module options apply only to module file.
func doCall(
	ctx context.Context,
	s *server.Server,
	moduleID string,
	moduleFile *os.File,
	moduleOpt *api.ModuleOptions,
	launch *api.LaunchOptions,
	suspendFD dbus.UnixFD,
	rFD dbus.UnixFD,
	wFD dbus.UnixFD,
	debugFD dbus.UnixFD,
	debugLogging bool,
) (string, api.State, api.Cause, int32) {
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

	syscall.SetNonblock(int(wFD), true)
	w := os.NewFile(uintptr(wFD), "w")
	defer w.Close()

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

	inst.Connect(ctx, r, w)
	status := inst.Wait(ctx)
	return inst.ID(), status.State, status.Cause, status.Result
}

// doLaunch module id or file.  Module options apply only to module file.
func doLaunch(
	ctx context.Context,
	s *server.Server,
	moduleID string,
	moduleFile *os.File,
	moduleOpt *api.ModuleOptions,
	launch *api.LaunchOptions,
	debugFD dbus.UnixFD,
	debugLogging bool,
) (inst api.Instance) {
	invoke, cancel := invokeOptions(debugFD, debugLogging)
	defer cancel()

	var err error
	if moduleFile != nil {
		upload := moduleUpload(moduleFile)
		defer upload.Close()
		inst, err = s.UploadModuleInstance(ctx, upload, moduleOpt, launch, invoke)
	} else {
		inst, err = s.NewInstance(ctx, moduleID, launch, invoke)
	}
	check(err)
	return
}

func listInstances(ctx context.Context, s *server.Server) []string {
	instances, err := s.Instances(ctx)
	check(err)
	sort.Sort(instances)
	ids := make([]string, 0, len(instances.Instances))
	for _, i := range instances.Instances {
		ids = append(ids, i.Instance)
	}
	return ids
}

func getInstanceInfo(ctx context.Context, s *server.Server, instanceID string) (state api.State, cause api.Cause, result int32, tags []string) {
	info, err := s.InstanceInfo(ctx, instanceID)
	check(err)
	return info.Status.State, info.Status.Cause, info.Status.Result, info.Tags
}

func waitInstance(ctx context.Context, s *server.Server, instanceID string) (state api.State, cause api.Cause, result int32) {
	status, err := s.WaitInstance(ctx, instanceID)
	check(err)
	return status.State, status.Cause, status.Result
}

func deleteInstance(ctx context.Context, s *server.Server, instanceID string) {
	check(s.DeleteInstance(ctx, instanceID))
}

func suspendInstance(ctx context.Context, s *server.Server, instanceID string) {
	_, err := s.SuspendInstance(ctx, instanceID)
	check(err)
}

func resumeInstance(ctx context.Context, s *server.Server, instance string, resume *api.ResumeOptions, debugFD dbus.UnixFD, debugLogging bool) {
	invoke, cancel := invokeOptions(debugFD, debugLogging)
	defer cancel()

	_, err := s.ResumeInstance(ctx, instance, resume, invoke)
	check(err)
}

func killInstance(ctx context.Context, s *server.Server, instanceID string) {
	_, err := s.KillInstance(ctx, instanceID)
	check(err)
}

func connectInstance(ctx context.Context, s *server.Server, instanceID string, rFD, wFD dbus.UnixFD) bool {
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
	check(err) // First SetNonblock error.

	_, connIO, err := s.InstanceConnection(ctx, instanceID)
	check(err)
	if connIO == nil {
		return false
	}

	check(connIO(ctx, r, w))
	return true
}

func snapshot(ctx context.Context, s *server.Server, instanceID string, moduleOpt *api.ModuleOptions) string {
	moduleID, err := s.Snapshot(ctx, instanceID, moduleOpt)
	check(err)
	return moduleID
}

func updateInstance(ctx context.Context, s *server.Server, instanceID string, update *api.InstanceUpdate) {
	_, err := s.UpdateInstance(ctx, instanceID, update)
	check(err)
}

func debugInstance(ctx context.Context, s *server.Server, instanceID string, reqBuf []byte) []byte {
	req := new(api.DebugRequest)
	check(proto.Unmarshal(reqBuf, req))

	res, err := s.DebugInstance(ctx, instanceID, req)
	check(err)

	resBuf, err := proto.Marshal(res)
	check(err)
	return resBuf
}

type access struct {
	server.PublicAccess
}

func (a *access) AuthorizeInstance(ctx context.Context, res *server.ResourcePolicy, inst *server.InstancePolicy) (context.Context, error) {
	ctx, err := a.PublicAccess.AuthorizeInstance(ctx, res, inst)
	if err != nil {
		return ctx, err
	}

	return authorizeScope(ctx)
}

func (a *access) AuthorizeProgramInstance(ctx context.Context, res *server.ResourcePolicy, prog *server.ProgramPolicy, inst *server.InstancePolicy) (context.Context, error) {
	ctx, err := a.PublicAccess.AuthorizeProgramInstance(ctx, res, prog, inst)
	if err != nil {
		return ctx, err
	}

	return authorizeScope(ctx)
}

func (a *access) AuthorizeProgramInstanceSource(ctx context.Context, res *server.ResourcePolicy, prog *server.ProgramPolicy, inst *server.InstancePolicy, src string) (context.Context, error) {
	ctx, err := a.PublicAccess.AuthorizeProgramInstanceSource(ctx, res, prog, inst, src)
	if err != nil {
		return ctx, err
	}

	return authorizeScope(ctx)
}

func authorizeScope(ctx context.Context) (context.Context, error) {
	if gatescope.ContextContains(ctx, system.Scope) {
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

	data, err := ioutil.ReadAll(f)
	check(err)

	return &api.ModuleUpload{
		Stream: ioutil.NopCloser(bytes.NewReader(data)),
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

func asBusError(x interface{}) *dbus.Error {
	if x == nil {
		return nil
	}
	return dbus.MakeFailedError(pan.Error(x))
}

func newHTTPHandler(api http.Handler, origin string) http.Handler {
	mux := http.NewServeMux()

	for _, static := range c.HTTP.Static {
		if !strings.HasPrefix(static.URI, "/") {
			check(fmt.Errorf("static HTTP URI does not start with slash: %q", static.URI))
		}
		if static.Path == "" {
			check(fmt.Errorf("filesystem path not specified for static HTTP URI: %q", static.URI))
		}
		if strings.HasSuffix(static.URI, "/") != strings.HasSuffix(static.Path, "/") {
			check(errors.New("static HTTP URI and filesystem path must both end in slash if one ends in slash"))
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

func check(err error) {
	pan.Check(err)
}
