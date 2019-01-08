// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"crypto/tls"
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
	"syscall"
	"time"

	"github.com/gorilla/handlers"
	"github.com/tsavola/confi"
	"github.com/tsavola/gate/image"
	"github.com/tsavola/gate/runtime"
	"github.com/tsavola/gate/server"
	"github.com/tsavola/gate/server/monitor"
	"github.com/tsavola/gate/server/monitor/webmonitor"
	"github.com/tsavola/gate/server/persistence/inmemory"
	"github.com/tsavola/gate/server/sshkeys"
	"github.com/tsavola/gate/server/webserver"
	"github.com/tsavola/gate/service"
	"github.com/tsavola/gate/service/origin"
	"github.com/tsavola/gate/service/plugin"
	"github.com/tsavola/gate/source/ipfs"
	"github.com/tsavola/gate/webapi"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
	"golang.org/x/net/netutil"
)

const (
	DefaultInstanceStore  = "memory"
	DefaultProgramStorage = "memory"
	DefaultIndexStatus    = http.StatusNotFound
)

const shutdownTimeout = 15 * time.Second

const (
	// stdio, syslog, runtime, listener, debug, autocert (guess)
	serverFileOverhead = 3 + 3 + 1 + 1 + 1 + 2

	// executable, process with debug
	forkFileOverhead = 1 + 6

	// guess
	pluginFileOverhead = 2

	// conn
	connFileOverhead = 1 + forkFileOverhead
)

var c = new(struct {
	Runtime runtime.Config

	Image struct {
		Filesystem string
		PageSize   int
	}

	Plugin struct {
		LibDir string
	}

	Service map[string]interface{}

	Server struct {
		server.Config
		InstanceStore  string
		ProgramStorage string
		MaxConns       int
		Debug          string
	}

	Access struct {
		Policy string

		Public struct{}

		SSH struct {
			AuthorizedKeys string
		}
	}

	Principal struct {
		server.AccessConfig
	}

	Source struct {
		IPFS struct {
			ipfs.Config
		}
	}

	HTTP struct {
		Net  string
		Addr string
		webserver.Config
		AccessLog string

		TLS struct {
			Enabled bool
			Domains []string
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
})

func parseConfig(flags *flag.FlagSet) {
	flags.Var(confi.FileReader(c), "f", "read TOML configuration file")
	flags.Var(confi.Assigner(c), "c", "set a configuration key (path.to.key=value)")
	flags.Parse(os.Args[1:])
}

func main() {
	log.SetFlags(0)

	var fileLimit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &fileLimit); err != nil {
		log.Fatal(err)
	}

	c.Runtime.MaxProcs = runtime.DefaultMaxProcs
	c.Runtime.LibDir = "lib/gate/runtime"
	c.Runtime.Cgroup.Title = runtime.DefaultCgroupTitle
	c.Image.PageSize = os.Getpagesize()
	c.Plugin.LibDir = "lib/gate/service"
	c.Server.InstanceStore = DefaultInstanceStore
	c.Server.ProgramStorage = DefaultProgramStorage
	c.Server.PreforkProcs = server.DefaultPreforkProcs
	c.Principal.AccessConfig = server.DefaultAccessConfig
	c.HTTP.Net = "tcp"
	c.HTTP.Addr = "localhost:8888"
	c.HTTP.TLS.Domains = []string{"example.invalid"}
	c.HTTP.Index.Status = DefaultIndexStatus
	c.ACME.CacheDir = "/var/lib/gate-server-acme"
	c.ACME.DirectoryURL = "https://acme-staging.api.letsencrypt.org/directory"
	c.Monitor.BufSize = monitor.DefaultBufSize
	c.Monitor.HTTP.Net = "tcp"
	c.Monitor.HTTP.StaticDir = "server/monitor/webmonitor"

	flags := flag.NewFlagSet("", flag.ContinueOnError)
	flags.SetOutput(ioutil.Discard)
	parseConfig(flags)

	plugins, err := plugin.List(c.Plugin.LibDir)
	if err != nil {
		log.Fatal(err)
	}

	c.Service = plugins.Config

	originConfig := origin.DefaultConfig
	c.Service["origin"] = &originConfig

	if c.Server.PreforkProcs <= 0 {
		c.Server.PreforkProcs = 1
	}

	constantFileOverhead := serverFileOverhead + forkFileOverhead*c.Server.PreforkProcs + pluginFileOverhead*len(c.Service)
	guessMaxConns := (int(fileLimit.Cur) - constantFileOverhead) / connFileOverhead

	if c.Server.MaxConns == 0 {
		if guessMaxConns <= 0 {
			log.Fatalf("file descriptor limit is too low (%d) or number of preforked processes is too high (%d)", fileLimit.Cur, c.Server.PreforkProcs)
		}
		c.Server.MaxConns = guessMaxConns
	} else if c.Server.MaxConns > guessMaxConns {
		log.Printf("maximum number of accepted connections (%d) exceeds estimated limit (%d) in regard to file descriptor limit (%d)", c.Server.MaxConns, guessMaxConns, fileLimit.Cur)
	}

	flag.Usage = confi.FlagUsage(nil, c)
	parseConfig(flag.CommandLine)

	ctx := context.Background()

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

	serviceConfig := &service.Config{
		Registry: new(service.Registry),
	}
	if err := plugins.InitServices(serviceConfig); err != nil {
		critLog.Fatal(err)
	}

	services := func() server.InstanceServices {
		o := origin.New(&originConfig)
		r := serviceConfig.Registry.Clone()
		r.Register(o)
		return server.NewInstanceServices(r, o)
	}

	if c.Monitor.HTTP.Addr != "" {
		if c.Monitor.HTTP.Origins == nil && strings.HasPrefix(c.Monitor.HTTP.Addr, "localhost:") {
			c.Monitor.HTTP.Origins = []string{"http://" + c.Monitor.HTTP.Addr}
		}

		monitor, handler := webmonitor.New(ctx, &c.Monitor.Config, &c.Monitor.HTTP.Config)
		c.Server.Monitor = server.MultiMonitor(c.Server.Monitor, monitor)

		listener, err := net.Listen(c.Monitor.HTTP.Net, c.Monitor.HTTP.Addr)
		if err != nil {
			critLog.Fatal(err)
		}

		server := http.Server{Handler: handler}
		go func() {
			critLog.Fatal(server.Serve(listener))
		}()
	}

	c.Server.Executor, err = runtime.NewExecutor(ctx, &c.Runtime)
	if err != nil {
		critLog.Fatal(err)
	}

	var fs *image.Filesystem
	if c.Image.Filesystem != "" {
		fs = image.NewFilesystem(c.Image.Filesystem, c.Image.PageSize)
	}

	switch c.Server.InstanceStore {
	case "memory":
		c.Server.Config.InstanceStore = image.Memory

	case "filesystem":
		c.Server.Config.InstanceStore = fs

	default:
		critLog.Fatalf("unknown server.instancestore option: %q", c.Server.InstanceStore)
	}

	switch c.Server.ProgramStorage {
	case "memory":
		c.Server.Config.ProgramStorage = image.Memory

	case "filesystem":
		c.Server.Config.ProgramStorage = fs

	default:
		critLog.Fatalf("unknown server.programstorage option: %q", c.Server.ProgramStorage)
	}

	switch c.Access.Policy {
	case "public":
		c.Principal.AccessConfig.Services = services
		c.Server.AccessPolicy = &server.PublicAccess{
			AccessConfig: c.Principal.AccessConfig,
		}

	case "ssh":
		accessKeys := &sshkeys.AuthorizedKeys{
			AccessConfig: c.Principal.AccessConfig,
			Services:     func(uid string) server.InstanceServices { return services() },
		}

		uid := strconv.Itoa(os.Getuid())

		filename := c.Access.SSH.AuthorizedKeys
		if filename == "" {
			home := os.Getenv("HOME")
			if home == "" {
				critLog.Fatalf("access.ssh.authorizedkeys option or $HOME required")
			}
			filename = path.Join(home, ".ssh", "authorized_keys")
		}

		if err := accessKeys.ParseFile(uid, filename); err != nil {
			critLog.Fatal(err)
		}

		c.Server.AccessPolicy = accessKeys

	default:
		critLog.Fatalf("unknown access.policy option: %q", c.Access.Policy)
	}

	switch c.Server.Debug {
	case "":

	case "stderr":
		c.Server.Config.Debug = os.Stderr

	default:
		critLog.Fatalf("unknown server.debug option: %q", c.Server.Debug)
	}

	if c.HTTP.Authority == "" {
		c.HTTP.Authority = strings.Split(c.HTTP.Addr, ":")[0]
	}
	c.HTTP.AccessState = inmemory.NewDefault()

	c.HTTP.ModuleSources = make(map[string]server.Source)
	if c.Source.IPFS.Configured() {
		c.HTTP.ModuleSources[ipfs.Source] = ipfs.New(&c.Source.IPFS.Config)
	}

	var (
		acmeCache  autocert.Cache
		acmeClient *acme.Client
	)
	if c.ACME.AcceptTOS {
		acmeCache = autocert.DirCache(c.ACME.CacheDir)
		acmeClient = &acme.Client{DirectoryURL: c.ACME.DirectoryURL}
	}

	c.HTTP.Server = server.New(ctx, &c.Server.Config)
	handler := newHTTPSHandler(webserver.NewHandler(ctx, "/", &c.HTTP.Config))

	if c.HTTP.AccessLog != "" {
		f, err := os.OpenFile(c.HTTP.AccessLog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
		if err != nil {
			critLog.Fatal(err)
		}
		defer f.Close()

		handler = handlers.LoggingHandler(f, handler)
	}

	l, err := net.Listen(c.HTTP.Net, c.HTTP.Addr)
	if err != nil {
		critLog.Fatal(err)
	}

	if n := c.Server.MaxConns; n > 0 {
		l = netutil.LimitListener(l, n)
	}

	s := http.Server{Handler: handler}

	go func() {
		<-c.Server.Executor.Dead()
		critLog.Print("executor died")

		ctx, cancel := context.WithTimeout(ctx, shutdownTimeout)
		defer cancel()

		if err := s.Shutdown(ctx); err != nil {
			critLog.Fatalf("shutdown: %v", err)
		}
	}()

	if c.HTTP.TLS.Enabled {
		if !c.ACME.AcceptTOS {
			critLog.Fatal("http.tls requires acme.accepttos")
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

		s.TLSConfig = &tls.Config{
			GetCertificate: m.GetCertificate,
			NextProtos:     []string{"h2", "http/1.1"},
		}
		l = tls.NewListener(l, s.TLSConfig)

		go func() {
			critLog.Fatal(http.ListenAndServe(":http", m.HTTPHandler(http.HandlerFunc(handleHTTP))))
		}()
	}

	critLog.Fatal(s.Serve(l))
}

func newHTTPSHandler(gate http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	status := http.StatusNotFound
	message := "not found"

	switch {
	case r.URL.Path == "/":
		status = c.HTTP.Index.Status
		message = ""

		if s := c.HTTP.Index.Location; s != "" {
			w.Header().Set("Location", s)
		}

	case r.URL.Path == webapi.Path || strings.HasPrefix(r.URL.Path, webapi.Path+"/"):
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
