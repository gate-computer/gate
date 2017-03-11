package main

import (
	"crypto/tls"
	"flag"
	"log"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/tsavola/wag/wasm"
	"golang.org/x/crypto/acme/autocert"

	"github.com/tsavola/gate/run"
	"github.com/tsavola/gate/server"
)

const (
	renewCertBefore = 30 * 24 * time.Hour

	memorySizeLimit = 256 * wasm.Page
	stackSize       = 16 * 4096
)

func main() {
	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	var (
		executor      = path.Join(dir, "bin/executor")
		loader        = path.Join(dir, "bin/loader")
		loaderSymbols = loader + ".symbols"
		addr          = "localhost:8888"
		letsencrypt   = false
		email         = ""
		acceptTOS     = false
		certCacheDir  = "/var/lib/gate-httpserver-letsencrypt"
	)

	flag.StringVar(&executor, "executor", executor, "filename")
	flag.StringVar(&loader, "loader", loader, "filename")
	flag.StringVar(&loaderSymbols, "loader-symbols", loaderSymbols, "filename")
	flag.StringVar(&addr, "addr", addr, "listening [address]:port")
	flag.BoolVar(&letsencrypt, "letsencrypt", letsencrypt, "enable automatic TLS; domain names should be listed after the options")
	flag.StringVar(&email, "email", email, "contact address for Let's Encrypt")
	flag.BoolVar(&acceptTOS, "accept-tos", acceptTOS, "accept Let's Encrypt's terms of service")
	flag.StringVar(&certCacheDir, "cert-cache-dir", certCacheDir, "certificate storage")
	flag.Parse()
	domains := flag.Args()

	env, err := run.NewEnvironment(executor, loader, loaderSymbols)
	if err != nil {
		log.Fatal(err)
	}

	e := server.Executor{
		MemorySizeLimit: memorySizeLimit,
		StackSize:       stackSize,
		Interfaces:      interfaces{},
		Env:             env,
		Log:             log.New(os.Stderr, "", 0),
	}

	http.Handle("/execute", e.Handler())
	http.Handle("/execute-custom", e.CustomHandler())

	if letsencrypt {
		if !acceptTOS {
			log.Fatal("-accept-tos option not set")
		}

		m := autocert.Manager{
			Prompt:      autocert.AcceptTOS,
			Cache:       autocert.DirCache(certCacheDir),
			HostPolicy:  autocert.HostWhitelist(domains...),
			RenewBefore: renewCertBefore,
			Email:       email,
		}

		s := http.Server{
			Addr: addr,
			TLSConfig: &tls.Config{
				GetCertificate: m.GetCertificate,
			},
		}

		err = s.ListenAndServeTLS("", "")
	} else {
		err = http.ListenAndServe(addr, nil)
	}

	log.Fatal(err)
}

type interfaces struct{}

func (interfaces) Info(name string) (info run.InterfaceInfo) {
	return
}

func (interfaces) Message([]byte, uint32) (found bool) {
	return
}
