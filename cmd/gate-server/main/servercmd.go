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
	"io/ioutil"
	"log"
	"log/syslog"
	"net"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"gate.computer/gate/image"
	"gate.computer/gate/internal/cmdconf"
	"gate.computer/gate/internal/services"
	"gate.computer/gate/internal/sys"
	"gate.computer/gate/runtime"
	"gate.computer/gate/runtime/system"
	"gate.computer/gate/server"
	"gate.computer/gate/server/database"
	"gate.computer/gate/server/database/sql"
	"gate.computer/gate/server/event"
	"gate.computer/gate/server/sshkeys"
	"gate.computer/gate/server/web"
	webapi "gate.computer/gate/server/web/api"
	"gate.computer/gate/server/web/router"
	"gate.computer/gate/service"
	"gate.computer/gate/service/origin"
	"gate.computer/gate/service/random"
	httpsource "gate.computer/gate/source/http"
	"gate.computer/gate/source/ipfs"
	"github.com/coreos/go-systemd/v22/daemon"
	"github.com/gorilla/handlers"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
	"import.name/confi"
)

const serverHeaderValue = "gate"

const shutdownTimeout = 15 * time.Second

const (
	DefaultExecutorCount  = 1
	DefaultImageStorage   = "filesystem"
	DefaultImageVarDir    = "/var/lib/gate/image"
	DefaultDatabaseDriver = "sqlite"
	DefaultInventoryDSN   = "file:/var/lib/gate/inventory.sqlite?cache=shared"
	DefaultNet            = "tcp"
	DefaultHTTPAddr       = "localhost:8080"
	DefaultACMECacheDir   = "/var/cache/gate/acme"
)

var Defaults = []string{
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
		VarDir           string
	}

	Inventory map[string]database.Config

	Service map[string]interface{}

	Server server.Config

	Access struct {
		Policy string

		Public struct{}

		SSH struct {
			AuthorizedKeys string
		}
	}

	Principal server.AccessConfig

	Source struct {
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
		web.Config
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
		Syslog  bool
		Verbose bool
	}
}

var c = new(Config)

func Main() {
	log.SetFlags(0)

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
	c.Image.VarDir = DefaultImageVarDir
	c.Inventory = database.NewInventoryConfigs()
	c.Service = service.Config()
	c.Principal = server.DefaultAccessConfig
	c.HTTP.Net = DefaultNet
	c.HTTP.Addr = DefaultHTTPAddr
	c.HTTP.AccessDB = database.NewNonceCheckerConfigs()
	c.ACME.CacheDir = DefaultACMECacheDir
	c.ACME.DirectoryURL = "https://acme-staging.api.letsencrypt.org/directory"

	flags := flag.NewFlagSet("", flag.ContinueOnError)
	flags.SetOutput(ioutil.Discard)
	cmdconf.Parse(c, flags, true, Defaults...)

	if defaultDB && len(c.Inventory) == 1 {
		driver, err := confi.Get(c, "inventory.sql.driver")
		if err != nil {
			panic(err)
		}
		if driver == DefaultDatabaseDriver {
			dsn, err := confi.Get(c, "inventory.sql.dsn")
			if err != nil {
				panic(err)
			}
			if dsn == "" {
				confi.MustSet(c, "inventory.sql.dsn", DefaultInventoryDSN)
			}
		}
	}

	originConfig := origin.DefaultConfig
	c.Service["origin"] = &originConfig

	randomConfig := random.DefaultConfig
	c.Service["random"] = &randomConfig

	flag.Usage = confi.FlagUsage(nil, c)
	cmdconf.Parse(c, flag.CommandLine, false, Defaults...)

	var (
		critLog *log.Logger
		errLog  *log.Logger
		infoLog *log.Logger
	)
	if c.Log.Syslog {
		tag := path.Base(os.Args[0])

		w, err := syslog.New(syslog.LOG_CRIT, tag)
		if err != nil {
			log.Fatal(err)
		}
		critLog = log.New(w, "", 0)

		w, err = syslog.New(syslog.LOG_ERR, tag)
		if err != nil {
			critLog.Fatal(err)
		}
		errLog = log.New(w, "", 0)

		if c.Log.Verbose {
			w, err = syslog.New(syslog.LOG_INFO, tag)
			if err != nil {
				critLog.Fatal(err)
			}
			infoLog = log.New(w, "", 0)
		}
	} else {
		critLog = log.New(os.Stderr, "", 0)
		errLog = critLog

		if c.Log.Verbose {
			infoLog = critLog
		}
	}
	c.Runtime.ErrorLog = errLog

	var monitor func(*event.Event, error)
	if infoLog != nil {
		monitor = server.ErrorEventLogger(errLog, infoLog)
	} else {
		monitor = server.ErrorLogger(errLog)
	}
	c.Server.Monitor = monitor
	c.HTTP.Monitor = monitor

	mux := http.NewServeMux()
	ctx := router.Context(context.Background(), mux)

	var err error
	c.Principal.Services, err = services.Init(ctx, &originConfig, &randomConfig)
	if err != nil {
		critLog.Fatal(err)
	}

	critLog.Fatal(main2(ctx, mux, critLog))
}

func main2(ctx context.Context, mux *http.ServeMux, critLog *log.Logger) error {
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
		fs, err = image.NewFilesystem(c.Image.VarDir)
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
			filename, err = cmdconf.JoinHome(".ssh/authorized_keys")
			if err != nil {
				return fmt.Errorf("access.ssh.authorizedkeys option required (%w)", err)
			}
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

	c.Server.ModuleSources = make(map[string]server.Source, len(c.Source.HTTP))
	for _, x := range c.Source.HTTP {
		if x.Name != "" && x.Configured() {
			c.Server.ModuleSources[path.Join("/", x.Name)] = httpsource.New(&x.Config)
		}
	}
	if c.Source.IPFS.Configured() {
		c.Server.ModuleSources[ipfs.Source] = ipfs.New(&c.Source.IPFS.Config)
	}

	serverImpl, err := server.New(ctx, &c.Server)
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
		c.HTTP.NonceStorage, err = accessDB.InitNonceChecker(ctx)
		if err != nil {
			return err
		}
	}

	handler := web.NewHandler("/", &c.HTTP.Config)
	mux.Handle(webapi.Path, handler)
	handler = newWebHandler(mux)

	if c.HTTP.AccessLog != "" {
		f, err := os.OpenFile(c.HTTP.AccessLog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
		if err != nil {
			return err
		}
		defer f.Close()

		handler = handlers.LoggingHandler(f, handler)
	}

	webServer := &http.Server{Handler: handler}

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

		acmeServer = &http.Server{Handler: m.HTTPHandler(newACMEHandler())}

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
			critLog.Printf("shutdown: %v", err)
		}

		if acmeServer != nil {
			if err := acmeServer.Shutdown(ctx); err != nil {
				critLog.Printf("shutdown: %v", err)
			}
		}

		if err := serverImpl.Shutdown(ctx); err != nil {
			critLog.Printf("shutdown: %v", err)
		}
	}()

	for _, e := range executors {
		e := e
		go func() {
			<-e.Dead()
			critLog.Print("executor died")
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

func newWebHandler(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", serverHeaderValue)
		mux.ServeHTTP(w, r)
	})
}

func newACMEHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", serverHeaderValue)
		writeResponse(w, r, http.StatusMisdirectedRequest, "http not supported")
	})
}

func writeResponse(w http.ResponseWriter, r *http.Request, status int, message string) {
	if !acceptsText(r) {
		w.WriteHeader(status)
		return
	}

	w.Header().Set(webapi.HeaderContentType, "text/plain")
	w.WriteHeader(status)
	fmt.Fprintln(w, message)
}

func acceptsText(r *http.Request) bool {
	headers := r.Header[webapi.HeaderAccept]
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
