// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

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
	goruntime "runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"gate.computer/gate/image"
	"gate.computer/gate/internal/bus"
	"gate.computer/gate/internal/cmdconf"
	"gate.computer/gate/internal/defaultlog"
	"gate.computer/gate/internal/services"
	"gate.computer/gate/principal"
	gateruntime "gate.computer/gate/runtime"
	"gate.computer/gate/scope/program/system"
	"gate.computer/gate/server"
	"gate.computer/gate/server/api"
	"gate.computer/gate/server/web"
	grpc "gate.computer/gate/service/grpc/config"
	"gate.computer/gate/service/origin"
	"gate.computer/gate/service/plugin"
	"gate.computer/wag/compile"
	"github.com/coreos/go-systemd/v22/daemon"
	dbus "github.com/godbus/dbus/v5"
	"github.com/tsavola/confi"
	"google.golang.org/protobuf/proto"
)

const (
	DefaultImageVarDir = ".gate/image" // Relative to home directory.
)

// Defaults are relative to home directory.
var Defaults = []string{
	".config/gate/daemon.toml",
	".config/gate/daemon.d/*.toml",
}

type Config struct {
	Runtime gateruntime.Config

	Image struct {
		VarDir string
	}

	Plugin struct {
		LibDir string
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

func main() {
	log.SetFlags(0)

	defer func() {
		x := recover()
		if err, ok := x.(error); ok {
			log.Fatal(err)
		}
		panic(x)
	}()

	os.Exit(mainResult())
}

func mainResult() int {
	c.Runtime = gateruntime.DefaultConfig
	c.Image.VarDir = cmdconf.JoinHome(DefaultImageVarDir)
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
	cmdconf.Parse(c, flags, true, Defaults...)

	plugins, err := plugin.OpenAll(c.Plugin.LibDir)
	check(err)
	c.Service = plugins.ServiceConfig

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

	c.Principal.Services, err = services.Init(context.Background(), plugins, originConfig, defaultlog.StandardLogger{})
	check(err)

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

	s, err := server.New(ctx, server.Config{
		ImageStorage:   storage,
		ProcessFactory: exec,
		AccessPolicy:   &access{server.PublicAccess{AccessConfig: c.Principal}},
	})
	check(err)
	defer s.Shutdown(ctx)

	httpDone := make(chan error, 1)
	if c.HTTP.Addr != "" {
		host, port, err := net.SplitHostPort(c.HTTP.Addr)
		check(err)
		if host == "" {
			panic(errors.New("HTTP hostname must be configured explicitly"))
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
			panic(errors.New("no HTTP origins configured"))
		}
		for _, origin := range c.HTTP.Origins {
			if origin != "" && origin != "null" {
				u, err := url.Parse(origin)
				check(err)
				verifyLoopbackHost("HTTP origin", u.Hostname())
			}
		}

		c.HTTP.Server = s
		apiHandler := web.NewHandlerWithUnsecuredLocalAuthorization("/", c.HTTP.Config)
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
			panic(fmt.Errorf("%s hostname %q resolves to non-loopback IP address: %s", errorDesc, host, ip))
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
		panic(errors.New("daemon initialization was aborted"))
	}

	methods := map[string]interface{}{
		"CallKey": func(key, function string, rFD, wFD, suspendFD, debugFD dbus.UnixFD, debugLogging bool, instTags, modTags, scope []string) (instance string, state api.State, cause api.Cause, result int32, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			instance, state, cause, result = handleCall(ctx, s(), nil, key, function, false, rFD, wFD, suspendFD, debugFD, debugLogging, instTags, modTags, scope)
			return
		},

		"CallFile": func(moduleFD dbus.UnixFD, function string, ref bool, rFD, wFD, suspendFD, debugFD dbus.UnixFD, debugLogging bool, instTags, modTags, scope []string) (instance string, state api.State, cause api.Cause, result int32, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			instance, state, cause, result = handleCall(ctx, s(), os.NewFile(uintptr(moduleFD), "module"), "", function, ref, rFD, wFD, suspendFD, debugFD, debugLogging, instTags, modTags, scope)
			return
		},

		"Debug": func(instance string, req []byte) (res []byte, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			res = handleInstanceDebug(ctx, s(), instance, req)
			return
		},

		"Delete": func(instance string) (err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			handleInstanceDelete(ctx, s(), instance)
			return
		},

		"Download": func(moduleFD dbus.UnixFD, key string) (moduleLen int64, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			module := os.NewFile(uintptr(moduleFD), "module")
			r, moduleLen := handleModuleDownload(ctx, s(), key)
			go func() {
				defer module.Close()
				defer r.Close()
				io.Copy(module, r)
			}()
			return
		},

		"ListInstances": func() (list []string, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			list = handleInstanceList(ctx, s())
			return
		},

		"GetInstanceInfo": func(instance string) (state api.State, cause api.Cause, result int32, tags []string, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			state, cause, result, tags = handleInstanceInfo(ctx, s(), instance)
			return
		},

		"Wait": func(instance string) (state api.State, cause api.Cause, result int32, tags []string, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			state, cause, result = handleInstanceWait(ctx, s(), instance)
			return
		},

		"IO": func(instance string, rFD, wFD dbus.UnixFD) (ok bool, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			ok = handleInstanceConnect(ctx, s(), instance, rFD, wFD)
			return
		},

		"LaunchKey": func(key, function string, suspend bool, debugFD dbus.UnixFD, debugLogging bool, instTags, modTags, scope []string) (instance string, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			instance = handleLaunch(ctx, s(), nil, key, function, false, suspend, debugFD, debugLogging, instTags, modTags, scope)
			return
		},

		"LaunchFile": func(moduleFD dbus.UnixFD, function string, ref, suspend bool, debugFD dbus.UnixFD, debugLogging bool, instTags, modTags, scope []string) (instance string, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			instance = handleLaunch(ctx, s(), os.NewFile(uintptr(moduleFD), "module"), "", function, ref, suspend, debugFD, debugLogging, instTags, modTags, scope)
			return
		},

		"ListModules": func() (list []string, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			list = handleModuleList(ctx, s())
			return
		},

		"GetModuleInfo": func(module string) (tags []string, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			tags = handleModuleInfo(ctx, s(), module)
			return
		},

		"Resume": func(instance, function string, debugFD dbus.UnixFD, debugLogging bool, scope []string) (err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			handleInstanceResume(ctx, s(), instance, function, debugFD, debugLogging, scope)
			return
		},

		"Snapshot": func(instance string) (module string, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			module = handleInstanceSnapshot(ctx, s(), instance)
			return
		},

		"Pin": func(key string, tags []string) (err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			handleModulePin(ctx, s(), key, tags)
			return
		},

		"Unpin": func(key string) (err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			handleModuleUnpin(ctx, s(), key)
			return
		},

		"Update": func(instance string, persist bool, tags []string) (err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			handleInstanceUpdate(ctx, s(), instance, persist, tags)
			return
		},

		"Upload": func(moduleFD dbus.UnixFD, moduleLen int64, key string, tags []string) (module string, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			module = handleModuleUpload(ctx, s(), os.NewFile(uintptr(moduleFD), "module"), moduleLen, key, tags)
			return
		},
	}

	for name, f := range map[string]instanceFunc{
		"Kill":    (*server.Server).KillInstance,
		"Suspend": (*server.Server).SuspendInstance,
	} {
		f := f // Closure needs a local copy of the iterator's current value.
		methods[name] = func(instance string) (err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			handleInstance(ctx, s(), f, instance)
			return
		}
	}

	return methods
}

func handleModuleList(ctx context.Context, s *server.Server) []string {
	refs, err := s.Modules(ctx)
	check(err)
	sort.Sort(refs)
	ids := make([]string, 0, len(refs.Modules))
	for _, ref := range refs.Modules {
		ids = append(ids, ref.Id)
	}
	return ids
}

func handleModuleInfo(ctx context.Context, s *server.Server, key string) (tags []string) {
	info, err := s.ModuleInfo(ctx, key)
	check(err)
	return info.Tags
}

func handleModuleDownload(ctx context.Context, s *server.Server, key string,
) (content io.ReadCloser, contentLen int64) {
	content, contentLen, err := s.ModuleContent(ctx, key)
	check(err)
	return
}

func handleModuleUpload(ctx context.Context, s *server.Server, module *os.File, moduleLen int64, key string, tags []string) string {
	upload := &server.ModuleUpload{
		Stream: module,
		Length: moduleLen,
		Hash:   key,
	}
	defer upload.Close()

	id, err := s.UploadModule(ctx, upload, moduleOptions(true, tags))
	check(err)
	return id
}

func handleModulePin(ctx context.Context, s *server.Server, key string, tags []string) {
	check(s.PinModule(ctx, key, &api.ModuleOptions{
		Pin:  true,
		Tags: tags,
	}))
}

func handleModuleUnpin(ctx context.Context, s *server.Server, key string) {
	check(s.UnpinModule(ctx, key))
}

func handleCall(ctx context.Context, s *server.Server, module *os.File, key, function string, ref bool, rFD, wFD, suspendFD, debugFD dbus.UnixFD, debugLogging bool, instTags, modTags, scope []string) (instance string, state api.State, cause api.Cause, result int32) {
	invoke := invokeOptions(debugFD, debugLogging)
	defer invoke.Close()

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

	upload := moduleUpload(module)
	defer upload.Close()

	ctx = server.ContextWithScope(ctx, scope)

	launch := &api.LaunchOptions{
		Function:  function,
		Tags:      instTags,
		Transient: true,
	}

	var inst *server.Instance
	if upload != nil {
		inst, err = s.UploadModuleInstance(ctx, upload, moduleOptions(ref, modTags), launch, invoke)
	} else {
		inst, err = s.NewInstance(ctx, key, launch, invoke)
	}
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

func handleLaunch(ctx context.Context, s *server.Server, module *os.File, key, function string, ref, suspend bool, debugFD dbus.UnixFD, debugLogging bool, instTags, modTags, scope []string) string {
	invoke := invokeOptions(debugFD, debugLogging)
	defer invoke.Close()

	upload := moduleUpload(module)
	defer upload.Close()

	ctx = server.ContextWithScope(ctx, scope)

	launch := &api.LaunchOptions{
		Function: function,
		Suspend:  suspend,
		Tags:     instTags,
	}

	var (
		inst *server.Instance
		err  error
	)
	if upload != nil {
		inst, err = s.UploadModuleInstance(ctx, upload, moduleOptions(ref, modTags), launch, invoke)
	} else {
		inst, err = s.NewInstance(ctx, key, launch, invoke)
	}
	check(err)

	return inst.ID
}

func handleInstanceList(ctx context.Context, s *server.Server) []string {
	instances, err := s.Instances(ctx)
	check(err)
	sort.Sort(instances)
	ids := make([]string, 0, len(instances.Instances))
	for _, i := range instances.Instances {
		ids = append(ids, i.Instance)
	}
	return ids
}

func handleInstanceInfo(ctx context.Context, s *server.Server, instance string) (state api.State, cause api.Cause, result int32, tags []string) {
	info, err := s.InstanceInfo(ctx, instance)
	check(err)
	return info.Status.State, info.Status.Cause, info.Status.Result, info.Tags
}

func handleInstanceWait(ctx context.Context, s *server.Server, instance string) (state api.State, cause api.Cause, result int32) {
	status, err := s.WaitInstance(ctx, instance)
	check(err)
	return status.State, status.Cause, status.Result
}

func handleInstanceDelete(ctx context.Context, s *server.Server, instance string) {
	check(s.DeleteInstance(ctx, instance))
}

func handleInstanceResume(ctx context.Context, s *server.Server, instance, function string, debugFD dbus.UnixFD, debugLogging bool, scope []string) {
	ctx = server.ContextWithScope(ctx, scope)
	resume := &api.ResumeOptions{Function: function}
	invoke := invokeOptions(debugFD, debugLogging)
	defer invoke.Close()

	_, err := s.ResumeInstance(ctx, instance, resume, invoke)
	check(err)
}

func handleInstanceConnect(ctx context.Context, s *server.Server, instance string, rFD, wFD dbus.UnixFD) bool {
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

	_, connIO, err := s.InstanceConnection(ctx, instance)
	check(err)
	if connIO == nil {
		return false
	}

	check(connIO(ctx, r, w))
	return true
}

func handleInstanceSnapshot(ctx context.Context, s *server.Server, instance string) string {
	module, err := s.SnapshotInstance(ctx, instance, &api.ModuleOptions{Pin: true})
	check(err)
	return module
}

func handleInstanceUpdate(ctx context.Context, s *server.Server, instance string, persist bool, tags []string) {
	_, err := s.UpdateInstance(ctx, instance, &api.InstanceUpdate{
		Persist: persist,
		Tags:    tags,
	})
	check(err)
}

func handleInstanceDebug(ctx context.Context, s *server.Server, instance string, reqBuf []byte) []byte {
	req := new(api.DebugRequest)
	check(proto.Unmarshal(reqBuf, req))

	res, err := s.DebugInstance(ctx, instance, req)
	check(err)

	resBuf, err := proto.Marshal(res)
	check(err)
	return resBuf
}

func handleInstance(ctx context.Context, s *server.Server, f instanceFunc, instance string) {
	_, err := f(s, ctx, instance)
	check(err)
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

func (a *access) AuthorizeProgramInstanceSource(ctx context.Context, res *server.ResourcePolicy, prog *server.ProgramPolicy, inst *server.InstancePolicy, src server.Source) (context.Context, error) {
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

func moduleUpload(f *os.File) *server.ModuleUpload {
	if f == nil {
		return nil
	}

	if info, err := f.Stat(); err == nil && info.Mode().IsRegular() {
		return &server.ModuleUpload{
			Stream: f,
			Length: info.Size(),
		}
	}

	data, err := ioutil.ReadAll(f)
	check(err)

	return &server.ModuleUpload{
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

func invokeOptions(debugFD dbus.UnixFD, debugLogging bool) *server.InvokeOptions {
	f := os.NewFile(uintptr(debugFD), "debug")
	if debugLogging {
		return &server.InvokeOptions{DebugLog: f}
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

func newHTTPHandler(api http.Handler, origin string) http.Handler {
	mux := http.NewServeMux()

	for _, static := range c.HTTP.Static {
		if !strings.HasPrefix(static.URI, "/") {
			panic(fmt.Errorf("static HTTP URI does not start with slash: %q", static.URI))
		}
		if static.Path == "" {
			panic(fmt.Errorf("filesystem path not specified for static HTTP URI: %q", static.URI))
		}
		if strings.HasSuffix(static.URI, "/") != strings.HasSuffix(static.Path, "/") {
			panic(errors.New("static HTTP URI and filesystem path must both end in slash if one ends in slash"))
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
	if err != nil {
		panic(err)
	}
}
