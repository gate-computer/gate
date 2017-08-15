package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"log"
	"log/syslog"
	"net"
	"net/http"
	"os"
	"path"
	"syscall"
	"time"

	"github.com/tsavola/gate/run"
	"github.com/tsavola/gate/server"
	"github.com/tsavola/gate/server/serverconfig"
	_ "github.com/tsavola/gate/service/defaults"
	"github.com/tsavola/gate/service/origin"
	"github.com/tsavola/gate/webserver"
	"golang.org/x/crypto/acme/autocert"
	"golang.org/x/net/netutil"
)

const (
	renewCertBefore = 30 * 24 * time.Hour
)

func main() {
	var nofile syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &nofile); err != nil {
		log.Fatal(err)
	}

	var (
		runconf = run.Config{
			MaxProcs:    run.DefaultMaxProcs,
			LibDir:      "lib",
			CgroupTitle: run.DefaultCgroupTitle,
		}
		serverconf = serverconfig.Config{
			MemorySizeLimit: serverconfig.DefaultMemorySizeLimit,
			StackSize:       serverconfig.DefaultStackSize,
			PreforkProcs:    serverconfig.DefaultPreforkProcs,
		}
		webconf = webserver.Config{
			MaxProgramSize: webserver.DefaultMaxProgramSize,
		}
		addr         = "localhost:8888"
		maxConns     = int(nofile.Cur/8 - 10)
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

	flag.IntVar(&runconf.MaxProcs, "max-procs", runconf.MaxProcs, "limit number of simultaneous programs")
	flag.StringVar(&runconf.DaemonSocket, "daemon-socket", runconf.DaemonSocket, "use containerd via unix socket")
	flag.UintVar(&runconf.CommonGid, "common-gid", runconf.CommonGid, "group id for file descriptor sharing")
	flag.UintVar(&runconf.ContainerCred.Uid, "container-uid", runconf.ContainerCred.Uid, "user id for bootstrapping executor")
	flag.UintVar(&runconf.ContainerCred.Gid, "container-gid", runconf.ContainerCred.Gid, "group id for bootstrapping executor")
	flag.UintVar(&runconf.ExecutorCred.Uid, "executor-uid", runconf.ExecutorCred.Uid, "user id for executing code")
	flag.UintVar(&runconf.ExecutorCred.Gid, "executor-gid", runconf.ExecutorCred.Gid, "group id for executing code")
	flag.StringVar(&runconf.LibDir, "libdir", runconf.LibDir, "path")
	flag.StringVar(&runconf.CgroupParent, "cgroup-parent", runconf.CgroupParent, "slice")
	flag.StringVar(&runconf.CgroupTitle, "cgroup-title", runconf.CgroupTitle, "prefix of dynamic name")
	flag.IntVar(&serverconf.MemorySizeLimit, "memory-size-limit", serverconf.MemorySizeLimit, "memory size limit")
	flag.IntVar(&serverconf.StackSize, "stack-size", serverconf.StackSize, "stack size")
	flag.IntVar(&serverconf.PreforkProcs, "prefork-procs", serverconf.PreforkProcs, "number of processes to create in advance")
	flag.IntVar(&webconf.MaxProgramSize, "max-program-size", webconf.MaxProgramSize, "maximum accepted WebAssembly module upload size")
	flag.StringVar(&addr, "addr", addr, "listening [address]:port")
	flag.IntVar(&maxConns, "max-conns", maxConns, "limit number of simultaneous connections")
	flag.BoolVar(&letsencrypt, "letsencrypt", letsencrypt, "enable automatic TLS; domain names should be listed after the options")
	flag.StringVar(&email, "email", email, "contact address for Let's Encrypt")
	flag.BoolVar(&acceptTOS, "accept-tos", acceptTOS, "accept Let's Encrypt's terms of service")
	flag.StringVar(&certCacheDir, "cert-cache-dir", certCacheDir, "certificate storage")
	flag.BoolVar(&syslogging, "syslog", syslogging, "send log messages to syslog instead of stderr")
	flag.BoolVar(&debug, "debug", debug, "write payload programs' debug output to stderr")

	flag.Parse()

	domains := flag.Args()

	ctx := context.Background()

	var (
		critLog *log.Logger
		infoLog serverconfig.Logger
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

	env, err := run.NewEnvironment(&runconf)
	if err != nil {
		critLog.Fatal(err)
	}

	serverconf.Env = env
	serverconf.Services = services
	serverconf.Log = infoLog

	if debug {
		serverconf.Debug = os.Stderr
	}

	state := server.NewState(ctx, &serverconf)
	handler := webserver.NewHandler(ctx, "/", state, &webconf)

	l, err := net.Listen("tcp", addr)
	if err != nil {
		critLog.Fatal(err)
	}

	if maxConns > 0 {
		l = netutil.LimitListener(l, maxConns)
	}

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

		l = tls.NewListener(l, &tls.Config{
			GetCertificate: m.GetCertificate,
		})
	}

	s := http.Server{
		Handler: handler,
	}
	critLog.Fatal(s.Serve(l))
}

func services(r io.Reader, w io.Writer) run.ServiceRegistry {
	return origin.CloneRegistryWith(nil, r, w)
}
