package server

import (
	"compress/gzip"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/websocket"
	api "github.com/tsavola/gate/server/serverapi"
)

func makeHexId(id uint64) string {
	return fmt.Sprintf("%016x", id)
}

func makeInstance(id uint64) api.Instance {
	return api.Instance{
		Id: makeHexId(id),
	}
}

func newProgram(id uint64, hash []byte) *api.Program {
	return &api.Program{
		Id:     makeHexId(id),
		SHA512: hex.EncodeToString(hash),
	}
}

// NewHandler uses http.Request's context for resources whose lifetimes are
// tied to requests.  The context specified here is used for other resources,
// which are merely created by requests.
func NewHandler(ctx context.Context, pattern string, s *State) http.Handler {
	var (
		prefix = strings.TrimRight(pattern, "/")
		mux    = http.NewServeMux()
	)

	{
		var (
			path   = prefix + "/load"
			allow  = joinHeader(http.MethodPost, http.MethodOptions)
			accept = joinHeader("application/wasm", "application/json")
		)

		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			writeCORS(w, r, allow, "Content-Type, X-Gate-Program-Sha512")

			switch r.Method {
			case http.MethodPost:
				if acceptsJSON(r) {
					switch getContentType(r) {
					case "application/wasm":
						handleLoadWasm(w, r, s)

					case "application/json":
						handleLoadJSON(w, r, s)

					default:
						writeUnsupportedMediaType(w, accept)
					}
				} else {
					w.WriteHeader(http.StatusNotAcceptable)
				}

			case http.MethodOptions:
				writeOptionsAccept(w, allow, accept)

			default:
				writeMethodNotAllowed(w, allow)
			}
		})
	}

	{
		var (
			path   = prefix + "/spawn"
			allow  = joinHeader(http.MethodPost, http.MethodOptions)
			accept = joinHeader("application/wasm", "application/json")
		)

		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			writeCORS(w, r, allow, "Content-Type, X-Gate-Program-Sha512")

			switch r.Method {
			case http.MethodPost:
				if acceptsJSON(r) {
					switch getContentType(r) {
					case "application/wasm":
						handleSpawnWasm(ctx, w, r, s)

					case "application/json":
						handleSpawnJSON(ctx, w, r, s)

					default:
						writeUnsupportedMediaType(w, accept)
					}
				} else {
					w.WriteHeader(http.StatusNotAcceptable)
				}

			case http.MethodOptions:
				writeOptionsAccept(w, allow, accept)

			default:
				writeMethodNotAllowed(w, allow)
			}
		})
	}

	{
		var (
			path      = prefix + "/run"
			allow     = joinHeader(http.MethodGet, http.MethodPost, http.MethodOptions)
			allowCORS = joinHeader(http.MethodPost, http.MethodOptions) // excluding websocket
		)

		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			writeCORSExposeHeaders(w, r, allowCORS, "Content-Type, TE, X-Gate-Program-Id, X-Gate-Program-Sha512", "X-Gate-Instance-Id")

			switch r.Method {
			case http.MethodGet:
				handleRunWebsocket(w, r, s)

			case http.MethodPost:
				handleRunPost(w, r, s)

			case http.MethodOptions:
				writeOptions(w, allow)

			default:
				writeMethodNotAllowed(w, allow)
			}
		})
	}

	{
		var (
			path      = prefix + "/communicate"
			allow     = joinHeader(http.MethodGet, http.MethodPost, http.MethodOptions)
			allowCORS = joinHeader(http.MethodPost, http.MethodOptions) // excluding websocket
		)

		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			writeCORS(w, r, allowCORS, "Content-Type, X-Gate-Instance-Id")

			switch r.Method {
			case http.MethodGet:
				handleCommunicateWebsocket(w, r, s)

			case http.MethodPost:
				handleCommunicatePost(w, r, s)

			case http.MethodOptions:
				writeOptions(w, allow)

			default:
				writeMethodNotAllowed(w, allow)
			}
		})
	}

	{
		var (
			path   = prefix + "/wait"
			allow  = joinHeader(http.MethodPost, http.MethodOptions)
			accept = "application/json"
		)

		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			writeCORS(w, r, allow, "Content-Type")

			switch r.Method {
			case http.MethodPost:
				if acceptsJSON(r) {
					switch getContentType(r) {
					case "application/json":
						handleWait(w, r, s)

					default:
						writeUnsupportedMediaType(w, accept)
					}
				} else {
					w.WriteHeader(http.StatusNotAcceptable)
				}

			case http.MethodOptions:
				writeOptionsAccept(w, allow, accept)

			default:
				writeMethodNotAllowed(w, allow)
			}
		})
	}

	return mux
}

func handleLoadWasm(w http.ResponseWriter, r *http.Request, s *State) {
	var (
		progHash []byte
		err      error
	)

	if s := r.Header.Get("X-Gate-Program-Sha512"); s != "" {
		progHash, err = hex.DecodeString(s)
		if err != nil {
			writeText(w, r, http.StatusBadRequest, "SHA-512 hash mismatch")
			return
		}
	}

	body := decodeContent(w, r, s)
	if body == nil {
		return
	}

	// TODO: size limit

	// upload method closes body to check for decoding errors

	progId, progHash, valid, err := s.upload(body, progHash)
	if err != nil {
		writeBadRequest(w, r, err) // TODO: don't leak sensitive information
		return
	}
	if !valid {
		writeText(w, r, http.StatusBadRequest, "SHA-512 hash mismatch")
		return
	}

	writeJSON(w, &api.Loaded{
		Program: &api.Program{
			Id:     fmt.Sprintf("%016x", progId),
			SHA512: hex.EncodeToString(progHash),
		},
	})
}

func handleLoadJSON(w http.ResponseWriter, r *http.Request, s *State) {
	var load api.Load

	if !decodeContentJSON(w, r, s, &load) {
		return
	}

	var (
		valid bool
		found bool
	)

	if progId, err := strconv.ParseUint(load.Program.Id, 16, 64); err == nil {
		if progHash, err := hex.DecodeString(load.Program.SHA512); err == nil {
			valid, found = s.check(progId, progHash)
		}
	}
	if !found {
		http.NotFound(w, r) // XXX: can't do this
		return
	}
	if !valid {
		writeText(w, r, http.StatusForbidden, "SHA-512 hash mismatch")
		return
	}

	writeJSON(w, &api.Loaded{})
}

func handleSpawnWasm(ctx context.Context, w http.ResponseWriter, r *http.Request, s *State) {
	// TODO: X-Gate-Program-Sha512 support

	in, out, originPipe := newPipe()

	ctx, cancel := context.WithCancel(ctx)
	// don't cancel in request scope

	body := decodeContent(w, r, s)
	if body == nil {
		cancel()
		return
	}

	// TODO: size limit

	// uploadAndInstantiate method closes body to check for decoding errors

	inst, instId, progId, progHash, err := s.uploadAndInstantiate(body, originPipe, cancel)
	if err != nil {
		writeBadRequest(w, r, err) // TODO: don't leak sensitive information
		cancel()
		return
	}

	go func() {
		defer cancel()
		defer out.Close()
		inst.run(ctx, &s.Settings, in, out)
	}()

	writeJSON(w, &api.Spawned{
		Loaded: api.Loaded{
			Program: newProgram(progId, progHash),
		},
		Instance: makeInstance(instId),
	})
}

func handleSpawnJSON(ctx context.Context, w http.ResponseWriter, r *http.Request, s *State) {
	var spawn api.Spawn

	if !decodeContentJSON(w, r, s, &spawn) {
		return
	}

	in, out, originPipe := newPipe()

	ctx, cancel := context.WithCancel(ctx)
	// don't cancel in request scope

	var (
		inst   *instance
		instId uint64
		valid  bool
		found  bool
	)

	if progId, err := strconv.ParseUint(spawn.Program.Id, 16, 64); err == nil {
		if progHash, err := hex.DecodeString(spawn.Program.SHA512); err == nil {
			inst, instId, valid, found, err = s.instantiate(progId, progHash, originPipe, cancel)
			if err != nil {
				writeBadRequest(w, r, err) // TODO: don't leak sensitive information
				cancel()
				return
			}
		}
	}
	if !found {
		http.NotFound(w, r) // XXX: can't do this
		cancel()
		return
	}
	if !valid {
		writeText(w, r, http.StatusForbidden, "SHA-512 hash mismatch")
		cancel()
		return
	}

	go func() {
		defer cancel()
		defer out.Close()
		inst.run(ctx, &s.Settings, in, out)
	}()

	writeJSON(w, &api.Spawned{
		Instance: makeInstance(instId),
	})
}

func handleRunWebsocket(w http.ResponseWriter, r *http.Request, s *State) {
	u := websocket.Upgrader{
		CheckOrigin: func(*http.Request) bool { return true },
	}

	conn, err := u.Upgrade(w, r, nil)
	if err != nil {
		s.Log.Printf("%s error: %v", r.RemoteAddr, err)
		return
	}
	defer conn.Close()

	frameType, frame, err := conn.NextReader()
	if err != nil {
		s.Log.Printf("%s error: %v", r.RemoteAddr, err)
		return
	}

	// TODO: size limit

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	var (
		inst   *instance
		instId uint64
		loaded api.Loaded
	)

	switch frameType {
	case websocket.BinaryMessage:
		var (
			progId   uint64
			progHash []byte
		)

		inst, instId, progId, progHash, err = s.uploadAndInstantiate(ioutil.NopCloser(frame), nil, cancel)
		if err != nil {
			// TODO
			return
		}

		loaded.Program = newProgram(progId, progHash)

	case websocket.TextMessage:
		var run api.Run

		err = json.NewDecoder(frame).Decode(&run)
		if err != nil {
			// TODO
			return
		}

		var (
			valid bool
			found bool
		)

		if progId, err := strconv.ParseUint(run.Program.Id, 16, 64); err == nil {
			if progHash, err := hex.DecodeString(run.Program.SHA512); err == nil {
				inst, instId, valid, found, err = s.instantiate(progId, progHash, nil, cancel)
				if err != nil {
					// TODO
					return
				}
			}
		}
		if !found {
			// TODO
			return
		}
		if !valid {
			// TODO
			return
		}
	}

	err = conn.WriteJSON(&api.Running{
		Spawned: api.Spawned{
			Loaded:   loaded,
			Instance: makeInstance(instId),
		},
	})
	if err != nil {
		s.abortInit(inst, instId)
		return
	}

	inst.run(ctx, &s.Settings, newWebsocketReader(conn), websocketWriter{conn})

	closeMsg := websocketNormalClosure

	if result, ok := s.waitInstance(inst, instId); ok {
		if result != nil {
			err = conn.WriteJSON(&api.Finished{
				Result: *result,
			})
			if err != nil {
				return
			}
		} else {
			closeMsg = websocketInternalServerErr
		}
	}

	conn.WriteMessage(websocket.CloseMessage, closeMsg)
}

func handleRunPost(w http.ResponseWriter, r *http.Request, s *State) {
	progHexId := r.Header.Get("X-Gate-Program-Id")
	if progHexId == "" {
		writeText(w, r, http.StatusBadRequest, "X-Gate-Program-Id header required")
		return
	}

	progHexHash := r.Header.Get("X-Gate-Program-Sha512")
	if progHexHash == "" {
		writeText(w, r, http.StatusBadRequest, "X-Gate-Program-Sha512 header required")
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	var (
		inst   *instance
		instId uint64
		valid  bool
		found  bool
	)

	if progId, err := strconv.ParseUint(progHexId, 16, 64); err == nil {
		if progHash, err := hex.DecodeString(progHexHash); err == nil {
			inst, instId, valid, found, err = s.instantiate(progId, progHash, nil, cancel)
			if err != nil {
				writeBadRequest(w, r, err) // TODO: don't leak sensitive information
				return
			}
		}
	}
	if !found {
		http.NotFound(w, r) // XXX: can't do this
		return
	}
	if !valid {
		writeText(w, r, http.StatusForbidden, "SHA-512 hash mismatch")
		return
	}

	w.Header().Set("X-Gate-Instance-Id", makeHexId(instId))

	inst.run(ctx, &s.Settings, r.Body, w)

	if result, ok := s.waitInstance(inst, instId); ok {
		switch {
		case result == nil:
			w.Header().Set(http.TrailerPrefix+"X-Gate-Internal-Error", "1")

		case result.Error != "":
			w.Header().Set(http.TrailerPrefix+"X-Gate-Result-Error", result.Error)

		case result.Trap != "":
			w.Header().Set(http.TrailerPrefix+"X-Gate-Result-Trap", result.Trap)

		default:
			w.Header().Set(http.TrailerPrefix+"X-Gate-Result-Exit", strconv.Itoa(result.Exit))
		}
	}
}

func handleCommunicateWebsocket(w http.ResponseWriter, r *http.Request, s *State) {
	u := websocket.Upgrader{
		CheckOrigin: func(*http.Request) bool { return true },
	}

	conn, err := u.Upgrade(w, r, nil)
	if err != nil {
		s.Log.Printf("%s error: %v", r.RemoteAddr, err)
		return
	}
	defer conn.Close()

	frameType, frame, err := conn.NextReader()
	if err != nil {
		s.Log.Printf("%s error: %v", r.RemoteAddr, err)
		return
	}

	if frameType != websocket.TextMessage {
		conn.WriteMessage(websocket.CloseMessage, websocketUnsupportedData)
		return
	}

	// TODO: size limit

	var communicate api.Communicate

	err = json.NewDecoder(frame).Decode(&communicate)
	if err != nil {
		// TODO
		return
	}

	var (
		originPipe *pipe
		found      bool
	)

	if instId, err := strconv.ParseUint(communicate.Instance.Id, 16, 64); err == nil {
		originPipe, found = s.attachOrigin(instId)
	}
	if !found {
		conn.WriteMessage(websocket.CloseMessage, websocketNormalClosure)
		return
	}
	if originPipe == nil {
		conn.WriteMessage(websocket.CloseMessage, websocketAlreadyCommunicating)
		return
	}

	err = conn.WriteJSON(api.Communicating{})
	if err != nil {
		return
	}

	originPipe.io(newWebsocketReader(conn), websocketWriter{conn})

	conn.WriteMessage(websocket.CloseMessage, websocketNormalClosure)
}

func handleCommunicatePost(w http.ResponseWriter, r *http.Request, s *State) {
	instHexId := r.Header.Get("X-Gate-Instance-Id")
	if instHexId == "" {
		writeText(w, r, http.StatusBadRequest, "X-Gate-Instance-Id header required")
		return
	}

	body := decodeContent(w, r, s)
	if body == nil {
		return
	}
	defer body.Close()

	var (
		originPipe *pipe
		found      bool
	)

	if instId, err := strconv.ParseUint(instHexId, 16, 64); err == nil {
		originPipe, found = s.attachOrigin(instId)
	}
	if !found {
		http.NotFound(w, r) // XXX: can't do this
		return
	}
	if originPipe == nil {
		writeText(w, r, http.StatusConflict, "Already communicating")
		return
	}

	originPipe.io(body, w)
}

func handleWait(w http.ResponseWriter, r *http.Request, s *State) {
	var wait api.Wait

	if !decodeContentJSON(w, r, s, &wait) {
		return
	}

	var (
		result *api.Result
		found  bool
	)

	if instId, err := strconv.ParseUint(wait.Instance.Id, 16, 64); err == nil {
		result, found = s.wait(instId)
	}
	if !found {
		http.NotFound(w, r) // XXX: can't do this
		return
	}
	if result == nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	writeJSON(w, &api.Finished{
		Result: *result,
	})
}

func joinHeader(fields ...string) string {
	return strings.Join(fields, ", ")
}

func acceptsJSON(r *http.Request) bool {
	return acceptsMediaType(r, "application/", "json")
}

func acceptsText(r *http.Request) bool {
	return acceptsMediaType(r, "text/", "plain")
}

func acceptsMediaType(r *http.Request, prefix, subtype string) bool {
	fields := r.Header["Accept"]
	if len(fields) == 0 {
		return true
	}

	for _, field := range fields {
		tokens := strings.SplitN(field, ";", 2)
		mediaType := strings.TrimSpace(tokens[0])

		if mediaType == "*/*" {
			return true
		}

		if strings.HasPrefix(mediaType, prefix) {
			tail := mediaType[len(prefix):]
			if tail == subtype || tail == "*" {
				return true
			}
		}
	}

	return false
}

func getContentType(r *http.Request) string {
	header := r.Header.Get("Content-Type")
	tokens := strings.SplitN(header, ";", 2)
	return strings.TrimSpace(tokens[0])
}

func decodeContent(w http.ResponseWriter, r *http.Request, s *State) io.ReadCloser {
	switch r.Header.Get("Content-Encoding") { // non-standard for request
	case "", "identity":
		return r.Body

	case "gzip":
		decoder, err := gzip.NewReader(r.Body)
		if err == nil {
			return decoder
		}

		s.Log.Printf("%v: %v", r.RemoteAddr, err)

	default:
		w.Header().Set("Accept-Encoding", "gzip") // non-standard for response
	}

	w.WriteHeader(http.StatusBadRequest)
	return nil
}

func decodeContentJSON(w http.ResponseWriter, r *http.Request, s *State, v interface{}) (ok bool) {
	if r.ContentLength < 0 {
		w.WriteHeader(http.StatusLengthRequired)
		return
	}

	body := decodeContent(w, r, s)
	if body == nil {
		return
	}

	// TODO: size limit

	if err := json.NewDecoder(body).Decode(v); err != nil {
		writeBadRequest(w, r, err)
		body.Close()
		return
	}

	if err := body.Close(); err != nil {
		writeBadRequest(w, r, err)
		return
	}

	ok = true
	return
}

func writeCORS(w http.ResponseWriter, r *http.Request, methods, headers string) (origin bool) {
	_, origin = r.Header["Origin"]
	if origin {
		w.Header().Set("Access-Control-Allow-Headers", headers)
		w.Header().Set("Access-Control-Allow-Methods", methods)
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Max-Age", "86400")
	}
	return
}

func writeCORSExposeHeaders(w http.ResponseWriter, r *http.Request, allowMethods, allowHeaders, exposeHeaders string) {
	if writeCORS(w, r, allowMethods, allowHeaders) {
		w.Header().Set("Access-Control-Expose-Headers", exposeHeaders)
	}
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	data, err := json.MarshalIndent(v, "", "\t")
	if err != nil {
		panic(err)
	}
	data = append(data, '\n')

	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Write(data)
}

func writeOptions(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
}

func writeOptionsAccept(w http.ResponseWriter, allow, accept string) {
	w.Header().Set("Accept", accept)          // non-standard for response
	w.Header().Set("Accept-Encoding", "gzip") // non-standard for response
	writeOptions(w, allow)
}

func writeMethodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	w.WriteHeader(http.StatusMethodNotAllowed)
}

func writeUnsupportedMediaType(w http.ResponseWriter, accept string) {
	w.Header().Set("Accept", accept)          // non-standard for response
	w.Header().Set("Accept-Encoding", "gzip") // non-standard for response
	w.WriteHeader(http.StatusUnsupportedMediaType)
}

func writeBadRequest(w http.ResponseWriter, r *http.Request, err error) {
	writeText(w, r, http.StatusBadRequest, err)
}

func writeText(w http.ResponseWriter, r *http.Request, status int, v ...interface{}) {
	if acceptsText(r) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(status)
		fmt.Fprintln(w, v...)
	} else {
		w.WriteHeader(status)
	}
}
