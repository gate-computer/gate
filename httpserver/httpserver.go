package main

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
	"github.com/tsavola/wag"
	"github.com/tsavola/wag/sections"
	"github.com/tsavola/wag/traps"
	"github.com/tsavola/wag/wasm"
	"golang.org/x/crypto/acme/autocert"

	"github.com/tsavola/gate/run"
)

type readWriter struct {
	io.Reader
	io.Writer
}

const (
	renewCertBefore = 30 * 24 * time.Hour

	memorySizeLimit = 256 * wasm.Page
	stackSize       = 16 * 4096
)

var webSocketClose = websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")

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

	env, err = run.NewEnvironment(executor, loader, loaderSymbols)
	if err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("/execute", handleExecute)
	http.HandleFunc("/execute-custom", handleExecuteCustom)

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

func handleExecute(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		handleExecuteWebSocket(w, r)

	case http.MethodPost:
		handleExecuteHTTP(w, r)

	default:
		r.Body.Close()
		w.Header().Set("Allow", "GET, POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func handleExecuteHTTP(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	log.Printf("%s begin", r.RemoteAddr)
	defer log.Printf("%s end", r.RemoteAddr)

	input := bufio.NewReader(r.Body)
	output := new(bytes.Buffer)

	exit, trap, err, internal := execute(r, input, input, output)
	if err != nil {
		log.Printf("%s error: %v", r.RemoteAddr, err)
		if internal {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintln(w, err)
		}
		return
	}

	if trap != 0 {
		w.Header().Set("X-Gate-Trap", trap.String())
	} else {
		w.Header().Set("X-Gate-Exit", strconv.Itoa(exit))
	}

	w.Write(output.Bytes())
}

func handleExecuteWebSocket(w http.ResponseWriter, r *http.Request) {
	u := websocket.Upgrader{
		CheckOrigin: func(*http.Request) bool { return true },
	}

	conn, err := u.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("%s error: %v", r.RemoteAddr, err)
		return
	}
	defer conn.Close()

	log.Printf("%s begin", r.RemoteAddr)
	defer log.Printf("%s end", r.RemoteAddr)

	_, wasm, err := conn.NextReader()
	if err != nil {
		log.Printf("%s error: %v", r.RemoteAddr, err)
		return
	}

	msg := make(map[string]interface{})

	exit, trap, err, internal := execute(r, bufio.NewReader(wasm), newWebSocketReader(conn), webSocketWriter{conn})
	if err != nil {
		log.Printf("%s error: %v", r.RemoteAddr, err)
		if internal {
			msg["error"] = "internal"
		} else {
			msg["error"] = err.Error()
		}
	} else if trap != 0 {
		msg["trap"] = trap.String()
	} else {
		msg["exit"] = exit
	}

	if conn.WriteJSON(msg) == nil {
		conn.WriteMessage(websocket.CloseMessage, webSocketClose)
	}
}

func handleExecuteCustom(w http.ResponseWriter, r *http.Request) {
	h, _ := w.(http.Hijacker)
	conn, rw, err := h.Hijack()
	if err != nil {
		log.Printf("%s error: %v", r.RemoteAddr, err)
		return
	}
	defer conn.Close()

	if _, err := rw.Write([]byte("HTTP/1.0 200 OK\r\n\r\n")); err != nil {
		log.Printf("%s error: %v", r.RemoteAddr, err)
		return
	}

	if err := rw.Flush(); err != nil {
		log.Printf("%s error: %v", r.RemoteAddr, err)
		return
	}

	log.Printf("%s begin", r.RemoteAddr)
	defer log.Printf("%s end", r.RemoteAddr)

	var wasmSize int64

	if err := binary.Read(rw, binary.LittleEndian, &wasmSize); err != nil {
		log.Printf("%s error: %v", r.RemoteAddr, err)
		return
	}

	wasm := bufio.NewReader(io.LimitReader(rw, wasmSize))

	_, _, err, _ = execute(r, wasm, rw, rw)
	if err != nil {
		log.Printf("%s error: %v", r.RemoteAddr, err)
	}
}

func execute(r *http.Request, wasm *bufio.Reader, input io.Reader, output io.Writer) (exit int, trap traps.Id, err error, internal bool) {
	var ns sections.NameSection

	m := wag.Module{
		UnknownSectionLoader: sections.UnknownLoaders{"name": ns.Load}.Load,
	}

	err = m.Load(wasm, env, nil, nil, run.RODataAddr, nil)
	if err != nil {
		return
	}

	_, memorySize := m.MemoryLimits()
	if memorySize > memorySizeLimit {
		memorySize = memorySizeLimit
	}

	payload, err := run.NewPayload(&m, memorySize, stackSize)
	if err != nil {
		return
	}
	defer payload.Close()

	exit, trap, err = run.Run(env, payload, readWriter{input, output})
	if err != nil {
		internal = true
	} else {
		err := payload.DumpStacktrace(os.Stderr, m.FunctionMap(), m.CallMap(), m.FunctionSignatures(), &ns)
		if err != nil {
			log.Printf("%s error: %v", r.RemoteAddr, err)
		}
	}
	return
}

type webSocketReader struct {
	conn  *websocket.Conn
	frame io.Reader
}

func newWebSocketReader(conn *websocket.Conn) *webSocketReader {
	return &webSocketReader{
		conn: conn,
	}
}

func (r *webSocketReader) Read(buf []byte) (n int, err error) {
	if r.frame == nil {
		_, r.frame, err = r.conn.NextReader()
		if err != nil {
			return
		}
	}

	n, err = r.frame.Read(buf)
	if err == io.EOF {
		r.frame = nil
		err = nil
	}
	return
}

type webSocketWriter struct {
	conn *websocket.Conn
}

func (w webSocketWriter) Write(buf []byte) (n int, err error) {
	err = w.conn.WriteMessage(websocket.BinaryMessage, buf)
	if err == nil {
		n = len(buf)
	}
	return
}
