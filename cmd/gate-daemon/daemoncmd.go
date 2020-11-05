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
	"github.com/coreos/go-systemd/v22/daemon"
	dbus "github.com/godbus/dbus/v5"
	"github.com/tsavola/confi"
	"github.com/tsavola/wag/compile"
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

type instanceStatusFunc func(*server.Server, context.Context, string) (*api.Status, error)
type instanceObjectFunc func(*server.Server, context.Context, string) (*server.Instance, error)

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
		"CallKey": func(key, function string, rFD, wFD, suspendFD, debugFD dbus.UnixFD, debugLogging bool, scope []string,
		) (instID string, state api.State, cause api.Cause, result int32, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			instID, state, cause, result = handleCall(ctx, s(), nil, key, function, false, rFD, wFD, suspendFD, debugFD, debugLogging, scope)
			return
		},

		"CallFile": func(moduleFD dbus.UnixFD, function string, ref bool, rFD, wFD, suspendFD, debugFD dbus.UnixFD, debugLogging bool, scope []string,
		) (instID string, state api.State, cause api.Cause, result int32, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			module := os.NewFile(uintptr(moduleFD), "module")
			defer module.Close()
			instID, state, cause, result = handleCall(ctx, s(), module, "", function, ref, rFD, wFD, suspendFD, debugFD, debugLogging, scope)
			return
		},

		"Debug": func(instID string, req []byte,
		) (res []byte, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			res = handleInstanceDebug(ctx, s(), instID, req)
			return
		},

		"Delete": func(instID string) (err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			handleInstanceDelete(ctx, s(), instID)
			return
		},

		"Download": func(moduleFD dbus.UnixFD, key string,
		) (moduleLen int64, err *dbus.Error) {
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

		"IO": func(instID string, rFD, wFD dbus.UnixFD) (ok bool, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			ok = handleInstanceConnect(ctx, s(), instID, rFD, wFD)
			return
		},

		"LaunchKey": func(key, function string, suspend bool, debugFD dbus.UnixFD, debugLogging bool, scope []string,
		) (instID string, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			instID = handleLaunch(ctx, s(), nil, key, function, false, suspend, debugFD, debugLogging, scope)
			return
		},

		"LaunchFile": func(moduleFD dbus.UnixFD, function string, ref, suspend bool, debugFD dbus.UnixFD, debugLogging bool, scope []string,
		) (instID string, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			module := os.NewFile(uintptr(moduleFD), "module")
			defer module.Close()
			instID = handleLaunch(ctx, s(), module, "", function, ref, suspend, debugFD, debugLogging, scope)
			return
		},

		"ListModuleRefs": func() (list []string, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			list = handleModuleList(ctx, s())
			return
		},

		"Resume": func(instID, function string, debugFD dbus.UnixFD, debugLogging bool, scope []string,
		) (err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			handleInstanceResume(ctx, s(), instID, function, debugFD, debugLogging, scope)
			return
		},

		"Snapshot": func(instID string) (progID string, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			progID = handleInstanceSnapshot(ctx, s(), instID)
			return
		},

		"Unref": func(key string) (err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			handleModuleUnref(ctx, s(), key)
			return
		},

		"Upload": func(moduleFD dbus.UnixFD, moduleLen int64, key string,
		) (progID string, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			module := os.NewFile(uintptr(moduleFD), "module")
			defer module.Close()
			progID = handleModuleUpload(ctx, s(), module, moduleLen, key)
			return
		},
	}

	for name, f := range map[string]instanceStatusFunc{
		"GetStatus": (*server.Server).InstanceStatus,
		"Wait":      (*server.Server).WaitInstance,
	} {
		f := f // Closure needs a local copy of the iterator's current value.
		methods[name] = func(instID string) (state api.State, cause api.Cause, result int32, err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			state, cause, result = handleInstanceStatus(ctx, s(), f, instID)
			return
		}
	}

	for name, f := range map[string]instanceObjectFunc{
		"Kill":    (*server.Server).KillInstance,
		"Suspend": (*server.Server).SuspendInstance,
	} {
		f := f // Closure needs a local copy of the iterator's current value.
		methods[name] = func(instID string) (err *dbus.Error) {
			defer func() { err = asBusError(recover()) }()
			handleInstanceObject(ctx, s(), f, instID)
			return
		}
	}

	return methods
}

func handleModuleList(ctx context.Context, s *server.Server) []string {
	refs, err := s.ModuleRefs(ctx)
	check(err)
	sort.Sort(refs)
	ids := make([]string, 0, len(refs.Modules))
	for _, ref := range refs.Modules {
		ids = append(ids, ref.Id)
	}
	return ids
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

func handleInstanceStatus(ctx context.Context, s *server.Server, f instanceStatusFunc, instID string,
) (state api.State, cause api.Cause, result int32) {
	status, err := f(s, ctx, instID)
	check(err)
	return status.State, status.Cause, status.Result
}

func handleInstanceObject(ctx context.Context, s *server.Server, f instanceObjectFunc, instID string) {
	_, err := f(s, ctx, instID)
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
	req := new(api.DebugRequest)
	check(proto.Unmarshal(reqBuf, req))

	res, err := s.DebugInstance(ctx, instID, req)
	check(err)

	resBuf, err = proto.Marshal(res)
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
