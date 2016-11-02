package main

import (
	"bufio"
	"crypto/tls"
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path"
	"time"

	"golang.org/x/crypto/acme/autocert"

	"github.com/tsavola/wag"
	"github.com/tsavola/wag/traps"
	"github.com/tsavola/wag/wasm"

	"github.com/tsavola/gate/run"
)

const (
	renewCertBefore = 30 * 24 * time.Hour

	memorySizeLimit = 16 * wasm.Page
	stackSize       = 16 * 4096
)

var env *run.Environment

func main() {
	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	var (
		executor      = path.Join(dir, "bin/executor")
		loader        = path.Join(dir, "bin/loader")
		loaderSymbols = loader + ".symbols"
		addr          = ":80"
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

	env, err = run.NewEnvironment(executor, loader, loaderSymbols)
	if err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("/execute", execute)

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

func execute(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	log.Printf("%s begin", r.RemoteAddr)
	defer log.Printf("%s end", r.RemoteAddr)

	wasm := bufio.NewReader(r.Body)

	var m wag.Module

	err := m.Load(wasm, env, nil, nil, run.RODataAddr, nil)
	if err != nil {
		log.Printf("%s error: %v", r.RemoteAddr, err)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintln(w, err)
		return
	}

	_, memorySize := m.MemoryLimits()
	if memorySize > memorySizeLimit {
		memorySize = memorySizeLimit
	}

	payload, err := run.NewPayload(&m, memorySize, stackSize)
	if err != nil {
		log.Printf("%s error: %v", r.RemoteAddr, err)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintln(w, err)
		return
	}
	defer payload.Close()

	output, err := run.Run(env, payload)
	if err != nil {
		if trap, ok := err.(traps.Id); ok {
			w.Header().Set("X-Gate-Trap", trap.String())
		} else {
			log.Printf("%s error: %v", r.RemoteAddr, err)
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintln(w, err)
			return
		}
	}

	var data []byte

	if len(output) >= 8 {
		size := binary.LittleEndian.Uint32(output)
		if size >= 8 && size <= uint32(len(output)) {
			data = output[8:size]
		}
	}

	if data == nil {
		log.Printf("%s error: invalid output: %v", r.RemoteAddr, output)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	w.WriteHeader(http.StatusOK)

	if _, err := w.Write(data); err != nil {
		log.Printf("%s error: %v", r.RemoteAddr, err)
	}
	return
}
