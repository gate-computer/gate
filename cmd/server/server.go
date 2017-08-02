package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"log"
	"log/syslog"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/tsavola/gate/run"
	"github.com/tsavola/gate/server"
	_ "github.com/tsavola/gate/service/defaults"
	"github.com/tsavola/gate/service/origin"
	"github.com/tsavola/wag/wasm"
	"golang.org/x/crypto/acme/autocert"
)

const (
	renewCertBefore = 30 * 24 * time.Hour

	memorySizeLimit = 256 * wasm.Page
	stackSize       = 16 * 4096
)

func main() {
	var (
		config = run.Config{
			MaxProcs:    run.DefaultMaxProcs,
			LibDir:      "lib",
			CgroupTitle: run.DefaultCgroupTitle,
		}
		addr         = "localhost:8888"
		letsencrypt  = false
		email        = ""
		acceptTOS    = false
		certCacheDir = "/var/lib/gate-server-letsencrypt"
		syslogging   = false
		debug        = false
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] [domain...]\nOptions:\n", os.Args[0])
		flag.PrintDefaults()
	}

	flag.IntVar(&config.MaxProcs, "max-procs", config.MaxProcs, "limit number of simultaneous programs")
	flag.StringVar(&config.DaemonSocket, "daemon-socket", config.DaemonSocket, "use containerd via unix socket")
	flag.UintVar(&config.CommonGid, "common-gid", config.CommonGid, "group id for file descriptor sharing")
	flag.UintVar(&config.ContainerCred.Uid, "container-uid", config.ContainerCred.Uid, "user id for bootstrapping executor")
	flag.UintVar(&config.ContainerCred.Gid, "container-gid", config.ContainerCred.Gid, "group id for bootstrapping executor")
	flag.UintVar(&config.ExecutorCred.Uid, "executor-uid", config.ExecutorCred.Uid, "user id for executing code")
	flag.UintVar(&config.ExecutorCred.Gid, "executor-gid", config.ExecutorCred.Gid, "group id for executing code")
	flag.StringVar(&config.LibDir, "libdir", config.LibDir, "path")
	flag.StringVar(&config.CgroupParent, "cgroup-parent", config.CgroupParent, "slice")
	flag.StringVar(&config.CgroupTitle, "cgroup-title", config.CgroupTitle, "prefix of dynamic name")
	flag.StringVar(&addr, "addr", addr, "listening [address]:port")
	flag.BoolVar(&letsencrypt, "letsencrypt", letsencrypt, "enable automatic TLS; domain names should be listed after the options")
	flag.StringVar(&email, "email", email, "contact address for Let's Encrypt")
	flag.BoolVar(&acceptTOS, "accept-tos", acceptTOS, "accept Let's Encrypt's terms of service")
	flag.StringVar(&certCacheDir, "cert-cache-dir", certCacheDir, "certificate storage")
	flag.BoolVar(&syslogging, "syslog", syslogging, "send log messages to syslog instead of stderr")
	flag.BoolVar(&debug, "debug", debug, "write payload programs' debug output to stderr")

	flag.Parse()

	domains := flag.Args()

	var (
		critLog *log.Logger
		infoLog server.Logger
	)

	if syslogging {
		tag := path.Base(os.Args[0])

		w, err := syslog.New(syslog.LOG_CRIT, tag)
		if err != nil {
			log.Fatal(err)
		}
		critLog = log.New(w, "", 0)

		w, err = syslog.New(syslog.LOG_INFO, tag)
		if err != nil {
			critLog.Fatal(err)
		}
		infoLog = log.New(w, "", 0)
	} else {
		critLog = log.New(os.Stderr, "", 0)
		infoLog = critLog
	}

	env, err := run.NewEnvironment(&config)
	if err != nil {
		critLog.Fatal(err)
	}
	defer env.Close()

	settings := server.Settings{
		MemorySizeLimit: memorySizeLimit,
		StackSize:       stackSize,
		Env:             env,
		Services:        services,
		Log:             infoLog,
	}

	if debug {
		settings.Debug = os.Stderr
	}

	state := server.NewState(settings)
	handler := server.NewHandler("/", state)

	if letsencrypt {
		if !acceptTOS {
			critLog.Fatal("-accept-tos option not set")
		}

		m := autocert.Manager{
			Prompt:      autocert.AcceptTOS,
			Cache:       autocert.DirCache(certCacheDir),
			HostPolicy:  autocert.HostWhitelist(domains...),
			RenewBefore: renewCertBefore,
			Email:       email,
		}

		s := http.Server{
			Addr:    addr,
			Handler: handler,
			TLSConfig: &tls.Config{
				GetCertificate: m.GetCertificate,
			},
		}

		err = s.ListenAndServeTLS("", "")
	} else {
		err = http.ListenAndServe(addr, handler)
	}

	critLog.Fatal(err)
}

func services(r io.Reader, w io.Writer) run.ServiceRegistry {
	return origin.CloneRegistryWith(nil, r, w)
}
