// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"crypto/tls"
	"flag"
	"log"
	"log/syslog"
	"net"
	"net/http"
	"os"
	"path"
	"strings"
	"syscall"
	"time"

	"github.com/tsavola/config"
	"github.com/tsavola/gate/run"
	"github.com/tsavola/gate/server"
	"github.com/tsavola/gate/server/monitor"
	"github.com/tsavola/gate/server/monitor/webmonitor"
	"github.com/tsavola/gate/server/webserver"
	"github.com/tsavola/gate/service/defaults"
	"github.com/tsavola/gate/service/origin"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
	"golang.org/x/net/netutil"
)

const (
	// stdio, syslog, runtime, listener, debug, autocert (guess)
	globalFileOverhead = 3 + 3 + 1 + 1 + 1 + 2

	// conn, image, process
	connFileOverhead = 1 + 1 + 3

	procInitParallelism = 10
	procInitFilePool    = (7 - connFileOverhead) * procInitParallelism
)

type Config struct {
	Runtime run.Config

	Server struct {
		server.Config
		MaxConns int
		Debug    string
	}

	HTTP struct {
		Net  string
		Addr string

		TLS struct {
			Enabled bool
			Domains []string
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

func main() {
	var (
		nofile syscall.Rlimit
	)
	err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &nofile)
	if err != nil {
		log.Fatal(err)
	}

	c := new(Config)

	c.Runtime.MaxProcs = run.DefaultMaxProcs
	c.Runtime.LibDir = "lib"
	c.Runtime.CgroupTitle = run.DefaultCgroupTitle
	c.Server.MaxProgramSize = server.DefaultMaxProgramSize
	c.Server.MemorySizeLimit = server.DefaultMemorySizeLimit
	c.Server.StackSize = server.DefaultStackSize
	c.Server.PreforkProcs = server.DefaultPreforkProcs
	c.Server.MaxConns = int(nofile.Cur-globalFileOverhead-procInitFilePool) / connFileOverhead
	c.HTTP.Net = "tcp"
	c.HTTP.Addr = "localhost:8888"
	c.HTTP.TLS.Domains = []string{"example.invalid"}
	c.ACME.CacheDir = "/var/lib/gate-server-acme"
	c.ACME.DirectoryURL = "https://acme-staging.api.letsencrypt.org/directory"
	c.Monitor.BufSize = monitor.DefaultBufSize
	c.Monitor.HTTP.Net = "tcp"
	c.Monitor.HTTP.StaticDir = "server/monitor/webmonitor"

	flag.Var(config.FileReader(c), "f", "read YAML configuration file")
	flag.Var(config.Assigner(c), "c", "set a configuration key (path.to.key=value)")
	flag.Usage = config.FlagUsage(c)
	flag.Parse()

	registry := defaults.Register(nil)
	c.Server.Services = func(s *server.Server) run.ServiceRegistry {
		r := registry.Clone()
		origin.New(s.Origin.R, s.Origin.W).Register(r)
		return r
	}

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
	c.Server.MonitorError = server.ErrorLogger(errLog)
	if infoLog != nil {
		c.Server.MonitorEvent = server.EventLogger(infoLog)
	}
	c.Monitor.HTTP.ErrorLog = errLog

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

	c.Runtime.FileLimiter = run.NewFileLimiter(int(nofile.Cur) - globalFileOverhead - c.Server.MaxConns)
	c.Server.Runtime, err = run.NewRuntime(ctx, &c.Runtime)
	if err != nil {
		critLog.Fatal(err)
	}

	go func() {
		<-c.Server.Runtime.Done()
		critLog.Fatal("executor died")
	}()

	switch c.Server.Debug {
	case "":

	case "unsafe-stderr":
		c.Server.Config.Debug = os.Stderr

	default:
		log.Fatalf("unknown server.debug option: %q", c.Server.Debug)
	}

	var (
		acmeCache  autocert.Cache
		acmeClient *acme.Client
	)
	if c.ACME.AcceptTOS {
		acmeCache = autocert.DirCache(c.ACME.CacheDir)
		acmeClient = &acme.Client{DirectoryURL: c.ACME.DirectoryURL}
	}

	state := server.NewState(ctx, &c.Server.Config)
	handler := webserver.NewHandler(ctx, "/", state)

	l, err := net.Listen(c.HTTP.Net, c.HTTP.Addr)
	if err != nil {
		critLog.Fatal(err)
	}

	if n := c.Server.MaxConns; n > 0 {
		l = netutil.LimitListener(l, n)
	}

	if c.HTTP.TLS.Enabled {
		if !c.ACME.AcceptTOS {
			log.Fatal("http.tls requires acme.accepttos")
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

		l = tls.NewListener(l, &tls.Config{GetCertificate: m.GetCertificate})
	}

	s := http.Server{Handler: handler}
	critLog.Fatal(s.Serve(l))
}
