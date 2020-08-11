// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"crypto/tls"
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
	"gate.computer/gate/runtime"
	"gate.computer/gate/server"
	"gate.computer/gate/server/database"
	_ "gate.computer/gate/server/database/sql"
	"gate.computer/gate/server/monitor"
	"gate.computer/gate/server/monitor/webmonitor"
	"gate.computer/gate/server/sshkeys"
	"gate.computer/gate/server/webserver"
	"gate.computer/gate/service/origin"
	"gate.computer/gate/service/plugin"
	"gate.computer/gate/source/ipfs"
	"gate.computer/gate/webapi"
	"github.com/coreos/go-systemd/v22/daemon"
	"github.com/gorilla/handlers"
	"github.com/tsavola/confi"
	"github.com/tsavola/listen"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
	"golang.org/x/net/netutil"
)

const serverHeaderValue = "gate"

const (
	DefaultExecutorCount   = 1
	DefaultProgramStorage  = "memory"
	DefaultInstanceStorage = "memory"
	DefaultImageVarDir     = "/var/lib/gate/image"
	DefaultIndexStatus     = http.StatusNotFound
	DefaultACMECacheDir    = "/var/cache/gate/acme"
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

	Plugin struct {
		LibDir string
	}

	Service map[string]interface{}

	DB map[string]interface{}

	Server struct {
		server.Config
		MaxConns int
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
		IPFS struct {
			ipfs.Config
		}
	}

	HTTP struct {
		Net  string
		Addr string
		webserver.Config
		AccessDB  string
		AccessLog string

		TLS struct {
			Enabled  bool
			Domains  []string
			HTTPAddr string
		}

		Index struct {
			Status   int
			Location string
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

	Monitor struct {
		monitor.Config

		HTTP struct {
			Net  string
			Addr string
			webmonitor.Config
		}
	}

	Log struct {
		Syslog  bool
		Verbose bool
	}
}

var c = new(Config)

const shutdownTimeout = 15 * time.Second

func main() {
	log.SetFlags(0)

	c.Runtime.Config = runtime.DefaultConfig
	c.Runtime.ExecutorCount = DefaultExecutorCount
	c.Image.ProgramStorage = DefaultProgramStorage
	c.Image.InstanceStorage = DefaultInstanceStorage
	c.Image.VarDir = DefaultImageVarDir
	c.Plugin.LibDir = plugin.DefaultLibDir
	c.Principal = server.DefaultAccessConfig
	c.HTTP.Net = "tcp"
	c.HTTP.Addr = "localhost:8888"
	c.HTTP.TLS.Domains = []string{"example.invalid"}
	c.HTTP.Index.Status = DefaultIndexStatus
	c.ACME.CacheDir = DefaultACMECacheDir
	c.ACME.DirectoryURL = "https://acme-staging.api.letsencrypt.org/directory"
	c.Monitor.BufSize = monitor.DefaultBufSize
	c.Monitor.HTTP.Net = "tcp"
	c.Monitor.HTTP.StaticDir = "server/monitor/webmonitor"

	flags := flag.NewFlagSet("", flag.ContinueOnError)
	flags.SetOutput(ioutil.Discard)
	cmdconf.Parse(c, flags, true, Defaults...)

	plugins, err := plugin.OpenAll(c.Plugin.LibDir)
	if err != nil {
		log.Fatal(err)
	}

	c.Service = plugins.ServiceConfig
	c.DB = database.DefaultConfig

	originConfig := origin.DefaultConfig
	c.Service["origin"] = &originConfig

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
	if infoLog != nil {
		c.Server.Monitor = server.ErrorEventLogger(errLog, infoLog)
	} else {
		c.Server.Monitor = server.ErrorLogger(errLog)
	}
	c.Monitor.HTTP.ErrorLog = errLog

	c.Principal.Services, err = services.Init(context.Background(), plugins, originConfig)
	if err != nil {
		critLog.Fatal(err)
	}

	critLog.Fatal(main2(critLog))
}

func main2(critLog *log.Logger) error {
	var err error

	ctx := context.Background()

	if c.Monitor.HTTP.Addr != "" {
		if c.Monitor.HTTP.Origins == nil && strings.HasPrefix(c.Monitor.HTTP.Addr, "localhost:") {
			c.Monitor.HTTP.Origins = []string{"http://" + c.Monitor.HTTP.Addr}
		}

		monitor, handler := webmonitor.New(ctx, c.Monitor.Config, c.Monitor.HTTP.Config)
		c.Server.Monitor = server.MultiMonitor(c.Server.Monitor, monitor)

		listener, err := net.Listen(c.Monitor.HTTP.Net, c.Monitor.HTTP.Addr)
		if err != nil {
			return err
		}

		server := http.Server{Handler: handler}
		go func() {
			critLog.Fatal(server.Serve(listener))
		}()
	}

	var executors []*runtime.Executor

	for i := 0; i < c.Runtime.ExecutorCount; i++ {
		e, err := runtime.NewExecutor(c.Runtime.Config)
		if err != nil {
			return err
		}
		executors = append(executors, e)
	}

	var factories []runtime.ProcessFactory

	for _, e := range executors {
		var f runtime.ProcessFactory = e
		if n := c.Runtime.PrepareProcesses; n > 0 {
			f = runtime.PrepareProcesses(ctx, f, n)
		}
		factories = append(factories, f)
	}

	c.Server.ProcessFactory = runtime.DistributeProcesses(factories...)

	var fs *image.Filesystem
	if c.Image.VarDir != "" {
		fs, err = image.NewFilesystem(c.Image.VarDir)
		if err != nil {
			return fmt.Errorf("filesystem: %v", err)
		}
		defer fs.Close()
	}

	var progStorage image.ProgramStorage
	var instStorage image.InstanceStorage

	switch s := c.Image.ProgramStorage; s {
	case "memory":
		progStorage = image.Memory

	case "filesystem":
		progStorage = fs

	default:
		return fmt.Errorf("unknown server.programstorage option: %q", s)
	}

	switch s := c.Image.InstanceStorage; s {
	case "memory":
		instStorage = image.Memory

	case "filesystem":
		instStorage = fs

	default:
		return fmt.Errorf("unknown server.instancestorage option: %q", s)
	}

	if n := c.Image.PreparePrograms; n > 0 {
		progStorage = image.PreparePrograms(ctx, progStorage, n)
	}
	if n := c.Image.PrepareInstances; n > 0 {
		instStorage = image.PrepareInstances(ctx, instStorage, n)
	}

	c.Server.Config.ImageStorage = image.CombinedStorage(progStorage, instStorage)

	switch c.Access.Policy {
	case "public":
		c.Server.AccessPolicy = &server.PublicAccess{AccessConfig: c.Principal}

	case "ssh":
		accessKeys := &sshkeys.AuthorizedKeys{AccessConfig: c.Principal}

		uid := strconv.Itoa(os.Getuid())

		filename := c.Access.SSH.AuthorizedKeys
		if filename == "" {
			filename = cmdconf.JoinHome(".ssh/authorized_keys")
			if filename == "" {
				return fmt.Errorf("access.ssh.authorizedkeys option or $HOME required")
			}
		}

		if err := accessKeys.ParseFile(uid, filename); err != nil {
			return err
		}

		c.Server.AccessPolicy = accessKeys

	default:
		return fmt.Errorf("unknown access.policy option: %q", c.Access.Policy)
	}

	if c.HTTP.Authority == "" {
		c.HTTP.Authority, _, err = net.SplitHostPort(c.HTTP.Addr)
		if err != nil {
			return err
		}
	}

	nonceChecker, err := database.OpenNonceChecker(ctx, c.HTTP.AccessDB, c.DB[c.HTTP.AccessDB])
	if err != nil {
		return err
	}
	defer nonceChecker.Close()
	c.HTTP.NonceStorage = nonceChecker

	c.HTTP.ModuleSources = make(map[string]server.Source)
	if c.Source.IPFS.Configured() {
		c.HTTP.ModuleSources[ipfs.Source] = ipfs.New(c.Source.IPFS.Config)
	}

	var (
		acmeCache  autocert.Cache
		acmeClient *acme.Client
	)
	if c.ACME.AcceptTOS {
		acmeCache = autocert.DirCache(c.ACME.CacheDir)
		acmeClient = &acme.Client{DirectoryURL: c.ACME.DirectoryURL}
	}

	c.HTTP.Server, err = server.New(ctx, c.Server.Config)
	if err != nil {
		return err
	}
	handler := newHTTPSHandler(webserver.NewHandler("/", c.HTTP.Config))

	if c.HTTP.AccessLog != "" {
		f, err := os.OpenFile(c.HTTP.AccessLog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
		if err != nil {
			return err
		}
		defer f.Close()

		handler = handlers.LoggingHandler(f, handler)
	}

	l, err := listen.Net(ctx, c.HTTP.Net, c.HTTP.Addr)
	if err != nil {
		return err
	}

	if n := c.Server.MaxConns; n > 0 {
		l = netutil.LimitListener(l, n)
	}

	httpServer := http.Server{Handler: handler}

	go func() {
		dead := make(chan struct{}, 1)

		for _, e := range executors {
			e := e
			go func() {
				<-e.Dead()
				select {
				case dead <- struct{}{}:
				default:
				}
			}()
		}

		<-dead
		critLog.Print("executor died")

		daemon.SdNotify(false, daemon.SdNotifyStopping)

		ctx, cancel := context.WithTimeout(ctx, shutdownTimeout)
		defer cancel()

		if err := httpServer.Shutdown(ctx); err != nil {
			critLog.Fatalf("shutdown: %v", err)
		}

		if err := c.HTTP.Server.Shutdown(ctx); err != nil {
			critLog.Fatalf("shutdown: %v", err)
		}
	}()

	if c.HTTP.TLS.Enabled {
		if !c.ACME.AcceptTOS {
			return errors.New("http.tls requires acme.accepttos")
		}

		m := &autocert.Manager{
			Prompt:      autocert.AcceptTOS,
			Cache:       acmeCache,
			HostPolicy:  autocert.HostWhitelist(c.HTTP.TLS.Domains...),
			RenewBefore: c.ACME.RenewBefore,
			Client:      acmeClient,
			Email:       c.ACME.Email,
			ForceRSA:    c.ACME.ForceRSA,
		}

		httpServer.TLSConfig = &tls.Config{
			GetCertificate: m.GetCertificate,
			NextProtos:     []string{"h2", "http/1.1"},
		}
		l = tls.NewListener(l, httpServer.TLSConfig)

		httpAddr := c.HTTP.TLS.HTTPAddr
		if !strings.Contains(httpAddr, ":") {
			httpAddr += ":http"
		}

		go func() {
			critLog.Fatal(http.ListenAndServe(httpAddr, m.HTTPHandler(http.HandlerFunc(handleHTTP))))
		}()
	}

	if _, err := daemon.SdNotify(false, daemon.SdNotifyReady); err != nil {
		critLog.Fatal(err)
	}

	return httpServer.Serve(l)
}

func newHTTPSHandler(gate http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", serverHeaderValue)

		if r.URL.Path == "/" {
			if s := c.HTTP.Index.Location; s != "" {
				w.Header().Set("Location", s)
			}
			w.WriteHeader(c.HTTP.Index.Status)
		} else {
			gate.ServeHTTP(w, r)
		}
	})
}

func handleHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Server", serverHeaderValue)

	status := http.StatusNotFound
	message := "not found"

	switch {
	case r.URL.Path == "/":
		status = c.HTTP.Index.Status
		message = ""

		if s := c.HTTP.Index.Location; s != "" {
			w.Header().Set("Location", s)
		}

	case strings.HasPrefix(r.URL.Path, webapi.Path):
		status = http.StatusMisdirectedRequest
		message = "HTTP scheme not supported"
	}

	if message != "" && acceptsText(r) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(status)
		fmt.Fprintln(w, message)
	} else {
		w.WriteHeader(status)
	}
}

func acceptsText(r *http.Request) bool {
	headers := r.Header["Accept"]
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
