package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/gorilla/websocket"
	"github.com/tsavola/wag"
	"github.com/tsavola/wag/sections"
	"github.com/tsavola/wag/traps"
	"github.com/tsavola/wag/wasm"

	"github.com/tsavola/gate"
	"github.com/tsavola/gate/run"
)

type Executor struct {
	MemorySizeLimit wasm.MemorySize
	StackSize       int32
	Env             *run.Environment
	Services        func(io.Reader, io.Writer) run.ServiceRegistry
	Log             Logger
	Debug           io.Writer
}

func (e *Executor) Handler() http.Handler {
	return http.HandlerFunc(e.handle)
}

func (e *Executor) handle(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		e.handleWebSocket(w, r)

	case http.MethodPost:
		e.handlePost(w, r)

	default:
		w.Header().Set("Allow", "GET, POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (e *Executor) handlePost(w http.ResponseWriter, r *http.Request) {
	input := bufio.NewReader(r.Body)
	output := new(bytes.Buffer)

	result, bug := e.executeResult(input, input, output, r)
	if bug {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	resultJson, err := json.Marshal(result)
	if err != nil {
		panic(err)
	}
	w.Header().Set(gate.ResultHTTPHeaderName, string(resultJson))

	if result.Error != "" {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintln(w, result.Error)
		return
	}

	w.Header().Set("Content-Length", strconv.Itoa(output.Len()))
	w.Header().Set("Content-Type", "application/octet-stream")
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

	_, wasm, err := conn.NextReader()
	if err != nil {
		e.Log.Printf("%s error: %v", r.RemoteAddr, err)
		return
	}

	result, bug := e.executeResult(bufio.NewReader(wasm), newWebSocketReader(conn), webSocketWriter{conn}, r)
	if bug {
		result.Error = "internal server error"
	}

	if conn.WriteJSON(result) == nil {
		conn.WriteMessage(websocket.CloseMessage, webSocketClose)
	}
}

func (e *Executor) executeResult(wasm *bufio.Reader, input io.Reader, output io.Writer, r *http.Request) (result gate.Result, internal bool) {
	exit, trap, err, internal := e.execute(wasm, input, output, r)

	switch {
	case internal:

	case err != nil:
		result.Error = err.Error()

	case trap != 0:
		result.Trap = trap.String()

	default:
		result.Exit = exit
	}

	return
}

func (e *Executor) execute(wasm *bufio.Reader, input io.Reader, output io.Writer, r *http.Request) (exit int, trap traps.Id, err error, bug bool) {
	e.Log.Printf("%s begin", r.RemoteAddr)
	defer e.Log.Printf("%s end", r.RemoteAddr)

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

	registry := e.Services(input, output)

	exit, trap, runErr := run.Run(e.Env, payload, registry, e.Debug)
	if runErr != nil {
		e.Log.Printf("%s error: %v", r.RemoteAddr, runErr)
		bug = true
	} else if (trap != 0 || exit != 0) && e.Debug != nil {
		if err := payload.DumpStacktrace(e.Debug, m.FunctionMap(), m.CallMap(), m.FunctionSignatures(), &ns); err != nil {
			e.Log.Printf("%s error: %v", r.RemoteAddr, err)
		}
	}
	return
}
