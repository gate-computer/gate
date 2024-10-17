// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"
	"crypto/tls"
	stdsql "database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"

	"gate.computer/gate/database"
	"gate.computer/gate/database/sql"
	"gate.computer/gate/image"
	"gate.computer/gate/runtime"
	"gate.computer/gate/runtime/system"
	"gate.computer/gate/server"
	"gate.computer/gate/server/sshkeys"
	"gate.computer/gate/server/tracelog"
	"gate.computer/gate/server/webserver"
	"gate.computer/gate/server/webserver/router"
	"gate.computer/gate/service"
	"gate.computer/gate/service/origin"
	"gate.computer/gate/service/random"
	"gate.computer/gate/source"
	httpsource "gate.computer/gate/source/http"
	"gate.computer/gate/source/ipfs"
	"gate.computer/gate/trace/httptrace"
	"gate.computer/gate/web"
	"gate.computer/internal/cmdconf"
	"gate.computer/internal/logging"
	"gate.computer/internal/services"
	"gate.computer/internal/sys"
	"github.com/coreos/go-systemd/v22/daemon"
	"github.com/gorilla/handlers"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
	"import.name/confi"

	. "import.name/pan/mustcheck"
	. "import.name/type/context"
)

const DefaultServerName = "gate"

const shutdownTimeout = 15 * time.Second

const (
	DefaultExecutorCount  = 1
	DefaultImageStorage   = "filesystem"
	DefaultImageStateDir  = "/var/lib/gate/image"
	DefaultDatabaseDriver = "sqlite"
	DefaultInventoryDSN   = "file:/var/lib/gate/inventory/inventory.sqlite?cache=shared"
	DefaultSourceCacheDSN = "file:/var/cache/gate/source/source.sqlite?cache=shared"
	DefaultNet            = "tcp"
	DefaultHTTPAddr       = "localhost:8080"
	DefaultACMECacheDir   = "/var/cache/gate/acme"
)

var DefaultConfigFiles = []string{
	"/etc/gate/server.toml",
	"/etc/gate/server.d/*.toml",
}

type Config struct {
	Runtime struct {
		runtime.Config
		PrepareProcesses int
		ExecutorCount    int
	}

	Image struct {
		ProgramStorage   string
		PreparePrograms  int
		InstanceStorage  string
		PrepareInstances int
		StateDir         string
	}

	Inventory map[string]database.Config

	Service map[string]any

	Server struct {
		server.Config

		UID int
		GID int
	}

	Access struct {
		Policy string

		Public struct{}

		SSH struct {
			AuthorizedKeys string
		}
	}

	Principal server.AccessConfig

	Source struct {
		Cache map[string]database.Config

		HTTP []struct {
			Name string
			httpsource.Config
		}

		IPFS struct {
			ipfs.Config
		}
	}

	HTTP struct {
		Net  string
		Addr string
		webserver.Config
		AccessDB  map[string]database.Config
		AccessLog string

		TLS struct {
			Enabled  bool
			Domains  []string
			HTTPNet  string
			HTTPAddr string
		}
	}

	ACME struct {
		AcceptTOS    bool
		CacheDir     string
		RenewBefore  time.Duration
		DirectoryURL string
		Email        string
		ForceRSA     bool
	}

	Log struct {
		Journal bool
	}
}

var c = new(Config)

var handlerFunc func(http.ResponseWriter, *http.Request, http.Handler)

// SetHandlerFunc replaces the function which is invoked for every HTTP
// request.  The registered function should call the next handler's ServeHTTP
// method to pass the control to the server.
//
// This function has no effect if called after during Main.
func SetHandlerFunc(f func(w http.ResponseWriter, r *http.Request, next http.Handler)) {
	handlerFunc = f
}

var extMux = http.NewServeMux()

// Router can be used to register additional URLs to serve via HTTP.
func Router() router.Router {
	return extMux
}

// Main will not return.
func Main() {
	drivers := stdsql.Drivers()
	defaultDB := len(drivers) == 1 && drivers[0] == DefaultDatabaseDriver && sql.DefaultConfig == (sql.Config{})
	if defaultDB {
		sql.DefaultConfig = sql.Config{
			Driver: DefaultDatabaseDriver,
		}
	}

	c.Runtime.Config = runtime.DefaultConfig
	c.Runtime.ExecutorCount = DefaultExecutorCount
	c.Image.ProgramStorage = DefaultImageStorage
	c.Image.InstanceStorage = DefaultImageStorage
	c.Image.StateDir = DefaultImageStateDir
	c.Inventory = database.NewInventoryConfigs()
	c.Service = service.Config()
	c.Principal = server.DefaultAccessConfig
	c.Source.Cache = database.NewSourceCacheConfigs()
	c.Source.IPFS.Do = httptrace.DoPropagate
	c.HTTP.Net = DefaultNet
	c.HTTP.Addr = DefaultHTTPAddr
	c.HTTP.AccessDB = database.NewNonceCheckerConfigs()
	c.ACME.CacheDir = DefaultACMECacheDir
	c.ACME.DirectoryURL = "https://acme-staging.api.letsencrypt.org/directory"

	flags := flag.NewFlagSet("", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	cmdconf.Parse(c, flags, true, DefaultConfigFiles...)

	if defaultDB {
		if len(c.Inventory) == 1 && Must(confi.Get(c, "inventory.sql.driver")) == DefaultDatabaseDriver && Must(confi.Get(c, "inventory.sql.dsn")) == "" {
			confi.MustSet(c, "inventory.sql.dsn", DefaultInventoryDSN)
		}
		if len(c.Source.Cache) == 1 && Must(confi.Get(c, "source.cache.sql.driver")) == DefaultDatabaseDriver && Must(confi.Get(c, "source.cache.sql.dsn")) == "" {
			confi.MustSet(c, "source.cache.sql.dsn", DefaultSourceCacheDSN)
		}
	}

	originConfig := origin.DefaultConfig
	c.Service["origin"] = &originConfig

	randomConfig := random.DefaultConfig
	c.Service["random"] = &randomConfig

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

	c.HTTP.StartSpan = tracelog.HTTPSpanStarter(log, "webserver: ")
	c.HTTP.AddEvent = tracelog.EventAdder(log, "webserver: ", nil)
	c.Server.StartSpan = tracelog.SpanStarter(log, "server: ")
	c.Server.AddEvent = tracelog.EventAdder(log, "server: ", nil)

	ctx := context.Background()

	c.Principal.Services, err = services.Init(router.Context(ctx, extMux), &originConfig, &randomConfig, log)
	if err != nil {
		log.ErrorContext(ctx, "service initialization failed", "error", err)
		os.Exit(1)
	}

	log.ErrorContext(ctx, "fatal error", "error", main2(ctx, log))
	os.Exit(1)
}

func main2(ctx Context, log *slog.Logger) error {
	var err error

	var (
		executors   []*runtime.Executor
		groupers    []runtime.GroupProcessFactory
		preparators []runtime.ProcessFactory
	)

	for i := 0; i < c.Runtime.ExecutorCount; i++ {
		e, err := runtime.NewExecutor(&c.Runtime.Config)
		if err != nil {
			return err
		}
		executors = append(executors, e)
		groupers = append(groupers, e)
	}

	for _, e := range executors {
		var f runtime.ProcessFactory = e
		if n := c.Runtime.PrepareProcesses; n > 0 {
			f = runtime.PrepareProcesses(ctx, f, n)
		}
		preparators = append(preparators, f)
	}

	c.Server.ProcessFactory = runtime.DistributeProcesses(preparators...)

	var fs *image.Filesystem
	if c.Image.ProgramStorage == "filesystem" || c.Image.InstanceStorage == "filesystem" {
		uid := -1
		gid := -1
		if c.Server.UID > 0 {
			uid = c.Server.UID
		}
		if c.Server.GID > 0 {
			gid = c.Server.GID
		}

		fs, err = image.NewFilesystemWithOwnership(c.Image.StateDir, uid, gid)
		if err != nil {
			return fmt.Errorf("filesystem: %w", err)
		}
		defer fs.Close()
	}

	var progStorage image.ProgramStorage
	switch s := c.Image.ProgramStorage; s {
	case "filesystem":
		progStorage = fs
	case "memory":
		progStorage = image.Memory
	default:
		return fmt.Errorf("unknown server.programstorage option: %q", s)
	}

	var instStorage image.InstanceStorage
	switch s := c.Image.InstanceStorage; s {
	case "filesystem":
		instStorage = fs
	case "memory":
		instStorage = image.Memory
	default:
		return fmt.Errorf("unknown server.instancestorage option: %q", s)
	}

	if n := c.Image.PreparePrograms; n > 0 {
		progStorage = image.PreparePrograms(ctx, progStorage, n)
	}
	if n := c.Image.PrepareInstances; n > 0 {
		instStorage = image.PrepareInstances(ctx, instStorage, n)
	}

	c.Server.ImageStorage = image.CombinedStorage(progStorage, instStorage)

	inventoryDB, err := database.Resolve(c.Inventory)
	if err != nil {
		return err
	}
	defer inventoryDB.Close()
	c.Server.Inventory, err = inventoryDB.InitInventory(ctx)
	if err != nil {
		return err
	}

	switch c.Access.Policy {
	case "public":
		c.Server.AccessPolicy = &server.PublicAccess{AccessConfig: c.Principal}

	case "ssh":
		accessKeys := &sshkeys.AuthorizedKeys{AccessConfig: c.Principal}

		uid := strconv.Itoa(os.Getuid())

		filename := c.Access.SSH.AuthorizedKeys
		if filename == "" {
			filename = cmdconf.ExpandEnv("${HOME}/.ssh/authorized_keys")
		}

		if err := accessKeys.ParseFile(uid, filename); err != nil {
			return err
		}

		c.Server.AccessPolicy = accessKeys

		grouper := runtime.DistributeGroupProcesses(groupers...)
		c.Server.ProcessFactory = system.GroupUserProcesses(grouper, c.Server.ProcessFactory)

	default:
		return fmt.Errorf("unknown access.policy option: %q", c.Access.Policy)
	}

	c.Server.ModuleSources = make(map[string]source.Source, len(c.Source.HTTP))
	for _, x := range c.Source.HTTP {
		if x.Name != "" && x.Configured() {
			c.Server.ModuleSources[path.Join("/", x.Name)] = httpsource.New(&x.Config)
		}
	}
	if c.Source.IPFS.Configured() {
		c.Server.ModuleSources[ipfs.Source] = tracelog.Source(ipfs.New(&c.Source.IPFS.Config), log)
	}

	sourceCacheDB, err := database.Resolve(c.Source.Cache)
	if err != nil {
		return err
	}
	defer sourceCacheDB.Close()
	c.Server.SourceCache, err = sourceCacheDB.InitSourceCache(ctx)
	if err != nil {
		return err
	}

	serverImpl, err := server.New(ctx, &c.Server.Config)
	if err != nil {
		return err
	}
	c.HTTP.Server = serverImpl

	if c.HTTP.Authority == "" {
		c.HTTP.Authority, _, err = net.SplitHostPort(c.HTTP.Addr)
		if err != nil {
			return fmt.Errorf("http.authority string cannot be inferred: %w", err)
		}
	}

	accessDB, err := database.Resolve(c.HTTP.AccessDB)
	if err != nil && err != database.ErrNoConfig {
		return err
	}
	if accessDB != nil {
		defer accessDB.Close()
		c.HTTP.NonceChecker, err = accessDB.InitNonceChecker(ctx)
		if err != nil {
			return err
		}
	}

	mux := http.NewServeMux()
	mux.Handle(web.Path, webserver.NewHandler("/", &c.HTTP.Config))
	mux.Handle("/", extMux)

	handler := newWebHandler(mux)

	if c.HTTP.AccessLog != "" {
		f, err := os.OpenFile(c.HTTP.AccessLog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o666)
		if err != nil {
			return err
		}
		defer f.Close()

		handler = handlers.LoggingHandler(f, handler)
	}

	webServer := &http.Server{
		Handler: wrapWithHandlerFunc(handler),
	}

	webListener, err := net.Listen(c.HTTP.Net, c.HTTP.Addr)
	if err != nil {
		return err
	}
	defer webListener.Close()

	var (
		acmeServer   *http.Server
		acmeListener net.Listener
	)
	if c.HTTP.TLS.Enabled {
		if !c.ACME.AcceptTOS {
			return errors.New("http.tls requires acme.accepttos")
		}

		m := &autocert.Manager{
			Prompt:      autocert.AcceptTOS,
			Cache:       autocert.DirCache(c.ACME.CacheDir),
			HostPolicy:  autocert.HostWhitelist(c.HTTP.TLS.Domains...),
			RenewBefore: c.ACME.RenewBefore,
			Client:      &acme.Client{DirectoryURL: c.ACME.DirectoryURL},
			Email:       c.ACME.Email,
			ForceRSA:    c.ACME.ForceRSA,
		}

		webServer.TLSConfig = &tls.Config{
			GetCertificate: m.GetCertificate,
			NextProtos:     []string{"h2", "http/1.1"},
		}
		webListener = tls.NewListener(webListener, webServer.TLSConfig)

		acmeServer = &http.Server{
			Handler: wrapWithHandlerFunc(m.HTTPHandler(newACMEHandler())),
		}

		if c.HTTP.TLS.HTTPNet == "" {
			c.HTTP.TLS.HTTPNet = c.HTTP.Net
		}

		acmeAddr := c.HTTP.TLS.HTTPAddr
		if !strings.Contains(acmeAddr, ":") {
			acmeAddr += ":http"
		}

		acmeListener, err = net.Listen(c.HTTP.TLS.HTTPNet, acmeAddr)
		if err != nil {
			return err
		}
		defer acmeListener.Close()
	}

	if c.Server.GID != 0 {
		if err := syscall.Setgid(c.Server.GID); err != nil {
			return err
		}
	}
	if c.Server.UID != 0 {
		if err := syscall.Setuid(c.Server.UID); err != nil {
			return err
		}
	}

	if err := sys.ClearCaps(); err != nil {
		return err
	}

	var (
		exit = make(chan error, 1)
		dead = make(chan struct{}, 1)
		done = make(chan struct{})
	)

	go func() {
		select {
		case exit <- webServer.Serve(webListener):
		default:
		}
	}()

	if acmeServer != nil {
		go func() {
			select {
			case exit <- acmeServer.Serve(acmeListener):
			default:
			}
		}()
	}

	if _, err := daemon.SdNotify(false, daemon.SdNotifyReady); err != nil {
		return err
	}

	go func() {
		defer close(done)
		<-dead
		daemon.SdNotify(false, daemon.SdNotifyStopping)

		ctx, cancel := context.WithTimeout(ctx, shutdownTimeout)
		defer cancel()

		if err := webServer.Shutdown(ctx); err != nil {
			log.ErrorContext(ctx, "web server shutdown failed", "error", err)
		}

		if acmeServer != nil {
			if err := acmeServer.Shutdown(ctx); err != nil {
				log.ErrorContext(ctx, "acme server shutdown failed", "error", err)
			}
		}

		if err := serverImpl.Shutdown(ctx); err != nil {
			log.ErrorContext(ctx, "server shutdown failed", "error", err)
		}
	}()

	for _, e := range executors {
		e := e
		go func() {
			<-e.Dead()
			log.ErrorContext(ctx, "executor died")
			select {
			case dead <- struct{}{}:
			default:
			}
		}()
	}

	err = <-exit

	select {
	case dead <- struct{}{}:
	default:
	}

	return err
}

func wrapWithHandlerFunc(next http.Handler) http.Handler {
	f := handlerFunc
	if f == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f(w, r, next)
	})
}

func setServerHeader(w http.ResponseWriter) {
	h := w.Header()
	if h.Get("Server") == "" {
		h.Set("Server", DefaultServerName)
	}
}

func newWebHandler(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setServerHeader(w)
		mux.ServeHTTP(w, r)
	})
}

func newACMEHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setServerHeader(w)
		writeResponse(w, r, http.StatusMisdirectedRequest, "http not supported")
	})
}

func writeResponse(w http.ResponseWriter, r *http.Request, status int, message string) {
	if !acceptsText(r) {
		w.WriteHeader(status)
		return
	}

	w.Header().Set(web.HeaderContentType, "text/plain")
	w.WriteHeader(status)
	fmt.Fprintln(w, message)
}

func acceptsText(r *http.Request) bool {
	headers := r.Header[web.HeaderAccept]
	if len(headers) == 0 {
		return true
	}

	for _, header := range headers {
		for _, field := range strings.Split(header, ",") {
			tokens := strings.SplitN(field, ";", 2)
			mediaType := strings.TrimSpace(tokens[0])

			switch mediaType {
			case "text/plain", "*/*", "text/*":
				return true
			}
		}
	}

	return false
}
