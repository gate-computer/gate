package server

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/gorilla/websocket"
	"github.com/tsavola/wag"
	"github.com/tsavola/wag/sections"
	"github.com/tsavola/wag/traps"
	"github.com/tsavola/wag/wasm"

	"github.com/tsavola/gate/run"
)

type Executor struct {
	MemorySizeLimit wasm.MemorySize
	StackSize       int32
	Env             *run.Environment
	Services        run.ServiceRegistry
	Log             Logger
}

func (e *Executor) Handler() http.Handler {
	return http.HandlerFunc(e.handle)
}

func (e *Executor) CustomHandler() http.Handler {
	return http.HandlerFunc(e.handleCustom)
}

func (e *Executor) handle(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		e.handleWebSocket(w, r)

	case http.MethodPost:
		e.handleHTTP(w, r)

	default:
		w.Header().Set("Allow", "GET, POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (e *Executor) handleHTTP(w http.ResponseWriter, r *http.Request) {
	e.Log.Printf("%s begin", r.RemoteAddr)
	defer e.Log.Printf("%s end", r.RemoteAddr)

	input := bufio.NewReader(r.Body)
	output := new(bytes.Buffer)

	exit, trap, err, internal := e.execute(input, input, output, r)
	if err != nil {
		e.Log.Printf("%s error: %v", r.RemoteAddr, err)
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

func (e *Executor) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	u := websocket.Upgrader{
		CheckOrigin: func(*http.Request) bool { return true },
	}

	conn, err := u.Upgrade(w, r, nil)
	if err != nil {
		e.Log.Printf("%s error: %v", r.RemoteAddr, err)
		return
	}
	defer conn.Close()

	e.Log.Printf("%s begin", r.RemoteAddr)
	defer e.Log.Printf("%s end", r.RemoteAddr)

	_, wasm, err := conn.NextReader()
	if err != nil {
		e.Log.Printf("%s error: %v", r.RemoteAddr, err)
		return
	}

	msg := make(map[string]interface{})

	exit, trap, err, internal := e.execute(bufio.NewReader(wasm), newWebSocketReader(conn), webSocketWriter{conn}, r)
	if err != nil {
		e.Log.Printf("%s error: %v", r.RemoteAddr, err)
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

func (e *Executor) handleCustom(w http.ResponseWriter, r *http.Request) {
	h, _ := w.(http.Hijacker)
	conn, rw, err := h.Hijack()
	if err != nil {
		e.Log.Printf("%s error: %v", r.RemoteAddr, err)
		return
	}
	defer conn.Close()

	if _, err := rw.Write([]byte("HTTP/1.0 200 OK\r\n\r\n")); err != nil {
		e.Log.Printf("%s error: %v", r.RemoteAddr, err)
		return
	}

	if err := rw.Flush(); err != nil {
		e.Log.Printf("%s error: %v", r.RemoteAddr, err)
		return
	}

	e.Log.Printf("%s begin", r.RemoteAddr)
	defer e.Log.Printf("%s end", r.RemoteAddr)

	var wasmSize int64

	if err := binary.Read(rw, binary.LittleEndian, &wasmSize); err != nil {
		e.Log.Printf("%s error: %v", r.RemoteAddr, err)
		return
	}

	wasm := bufio.NewReader(io.LimitReader(rw, wasmSize))

	_, _, err, _ = e.execute(wasm, rw, rw, r)
	if err != nil {
		e.Log.Printf("%s error: %v", r.RemoteAddr, err)
	}
}

func (e *Executor) execute(wasm *bufio.Reader, input io.Reader, output io.Writer, r *http.Request) (exit int, trap traps.Id, err error, internal bool) {
	var ns sections.NameSection

	m := wag.Module{
		MainSymbol:           "main",
		UnknownSectionLoader: sections.UnknownLoaders{"name": ns.Load}.Load,
	}

	err = m.Load(wasm, e.Env, new(bytes.Buffer), nil, run.RODataAddr, nil)
	if err != nil {
		return
	}

	_, memorySize := m.MemoryLimits()
	if memorySize > e.MemorySizeLimit {
		memorySize = e.MemorySizeLimit
	}

	payload, err := run.NewPayload(&m, memorySize, e.StackSize)
	if err != nil {
		return
	}
	defer payload.Close()

	exit, trap, err = run.Run(e.Env, payload, readWriter{input, output}, e.Services, nil)
	if err != nil {
		internal = true
	} else if trap != 0 || exit != 0 {
		err := payload.DumpStacktrace(os.Stderr, m.FunctionMap(), m.CallMap(), m.FunctionSignatures(), &ns)
		if err != nil {
			e.Log.Printf("%s error: %v", r.RemoteAddr, err)
		}
	}
	return
}
