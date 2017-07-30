package server

import (
	"compress/gzip"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/websocket"

	"github.com/tsavola/gate"
)

func makeInstance(id uint64) gate.Instance {
	return gate.Instance{
		Id: fmt.Sprintf("%016x", id),
	}
}

func newProgram(id uint64, hash []byte) *gate.Program {
	return &gate.Program{
		Id:     fmt.Sprintf("%016x", id),
		SHA512: hex.EncodeToString(hash),
	}
}

func NewHandler(pattern string, s *State) http.Handler {
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
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Content-Sha512-Hex")
			w.Header().Set("Access-Control-Allow-Methods", allow)
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Max-Age", "86400")

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
			path   = prefix + "/run"
			allow  = joinHeader(http.MethodPost, http.MethodOptions)
			accept = joinHeader("application/wasm", "application/json")
		)

		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Content-Sha512-Hex")
			w.Header().Set("Access-Control-Allow-Methods", allow)
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Max-Age", "86400")

			switch r.Method {
			case http.MethodPost:
				if acceptsJSON(r) {
					switch getContentType(r) {
					case "application/wasm":
						handleRunWasm(w, r, s)

					case "application/json":
						handleRunJSON(w, r, s)

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
			path  = prefix + "/run-origin-wait"
			allow = joinHeader(http.MethodGet, http.MethodOptions)
		)

		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				handleRunOriginWait(w, r, s)

			case http.MethodOptions:
				writeOptionsWebsocket(w, r, allow)

			default:
				writeMethodNotAllowed(w, allow)
			}
		})
	}

	{
		var (
			path  = prefix + "/origin"
			allow = joinHeader(http.MethodGet, http.MethodOptions)
		)

		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				handleOriginWebsocket(w, r, s)

			case http.MethodOptions:
				writeOptionsWebsocket(w, r, allow)

			default:
				writeMethodNotAllowed(w, allow)
			}
		})
	}

	{
		var (
			path  = prefix + "/origin/"
			allow = joinHeader(http.MethodPost, http.MethodOptions)
		)

		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Methods", allow)
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Max-Age", "86400")

			switch r.Method {
			case http.MethodPost:
				if acceptsJSON(r) {
					handleOriginPost(w, r, s, r.URL.Path[len(path):])
				} else {
					w.WriteHeader(http.StatusNotAcceptable)
				}

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
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", allow)
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Max-Age", "86400")

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

	if s := r.Header.Get("X-Content-Sha512-Hex"); s != "" { // non-standard
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

	writeJSON(w, &gate.Loaded{
		Program: &gate.Program{
			Id:     fmt.Sprintf("%016x", progId),
			SHA512: hex.EncodeToString(progHash),
		},
	})
}

func handleLoadJSON(w http.ResponseWriter, r *http.Request, s *State) {
	var load gate.Load

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

	writeJSON(w, &gate.Loaded{})
}

func handleRunWasm(w http.ResponseWriter, r *http.Request, s *State) {
	// TODO: X-Content-Sha512-Hex support

	body := decodeContent(w, r, s)
	if body == nil {
		return
	}

	// TODO: size limit

	// uploadAndInstantiate method closes body to check for decoding errors

	in, out, originSocket := newPipe()

	inst, instId, progId, progHash, err := s.uploadAndInstantiate(body, nil, originSocket)
	if err != nil {
		writeBadRequest(w, r, err) // TODO: don't leak sensitive information
		return
	}

	go inst.run(&s.Settings, in, out)

	writeJSON(w, &gate.Running{
		Instance: makeInstance(instId),
		Program:  newProgram(progId, progHash),
	})
}

func handleRunJSON(w http.ResponseWriter, r *http.Request, s *State) {
	var run gate.Run

	if !decodeContentJSON(w, r, s, &run) {
		return
	}

	var (
		inst   *instance
		instId uint64
		valid  bool
		found  bool
	)

	in, out, originSocket := newPipe()

	if progId, err := strconv.ParseUint(run.Program.Id, 16, 64); err == nil {
		if progHash, err := hex.DecodeString(run.Program.SHA512); err == nil {
			inst, instId, valid, found, err = s.instantiate(progId, progHash, nil, originSocket)
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

	go inst.run(&s.Settings, in, out)

	writeJSON(w, &gate.Running{
		Instance: makeInstance(instId),
	})
}

func handleRunOriginWait(w http.ResponseWriter, r *http.Request, s *State) {
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

	var (
		exited  = make(chan *gate.Result, 1)
		written = make(chan struct{})
		inst    *instance
		instId  uint64
		prog    *gate.Program
	)

	switch frameType {
	case websocket.BinaryMessage:
		var (
			progId   uint64
			progHash []byte
		)

		inst, instId, progId, progHash, err = s.uploadAndInstantiate(ioutil.NopCloser(frame), exited, nil)
		if err != nil {
			// TODO
			return
		}

		prog = newProgram(progId, progHash)

	case websocket.TextMessage:
		var run gate.Run

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
				inst, instId, valid, found, err = s.instantiate(progId, progHash, nil, nil)
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

	err = conn.WriteJSON(&gate.Running{
		Instance: makeInstance(instId),
		Program:  prog,
	})
	if err != nil {
		s.cancel(inst, instId)
		return
	}

	go inst.run(&s.Settings, newWebsocketReader(conn), newWebsocketWriteCloser(conn, written))

	result, _ := <-exited
	<-written

	if result == nil {
		conn.WriteMessage(websocket.CloseMessage, websocketInternalServerErr)
		return
	}

	err = conn.WriteJSON(&gate.Finished{
		Result: *result,
	})
	if err != nil {
		return
	}

	conn.WriteMessage(websocket.CloseMessage, websocketNormalClosure)
}

func handleOriginWebsocket(w http.ResponseWriter, r *http.Request, s *State) {
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

	var origin gate.Origin

	err = json.NewDecoder(frame).Decode(&origin)
	if err != nil {
		// TODO
		return
	}

	var (
		ok    bool
		found bool
	)

	if instId, err := strconv.ParseUint(origin.Instance.Id, 16, 64); err == nil {
		ok, found = s.attachOrigin(instId, newWebsocketReader(conn), websocketWriter{conn})
	}
	if !found {
		conn.WriteMessage(websocket.CloseMessage, websocketNormalClosure)
		return
	}
	if !ok {
		conn.WriteMessage(websocket.CloseMessage, websocketAlreadyAttached)
		return
	}

	err = conn.WriteJSON(struct{}{}) // success
	if err != nil {
		return
	}

	conn.WriteMessage(websocket.CloseMessage, websocketNormalClosure)
}

func handleOriginPost(w http.ResponseWriter, r *http.Request, s *State, urlSuffix string) {
	body := decodeContent(w, r, s)
	if body == nil {
		return
	}
	defer body.Close()

	var (
		ok    bool
		found bool
	)

	if instId, err := strconv.ParseUint(urlSuffix, 16, 64); err == nil {
		ok, found = s.attachOrigin(instId, body, w)
	}
	if !found {
		http.NotFound(w, r) // XXX: can't do this
		return
	}
	if !ok {
		writeText(w, r, http.StatusConflict, "Already attached")
		return
	}
}

func handleWait(w http.ResponseWriter, r *http.Request, s *State) {
	var wait gate.Wait

	if !decodeContentJSON(w, r, s, &wait) {
		return
	}

	var (
		result *gate.Result
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

	writeJSON(w, &gate.Finished{
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
	header := r.Header.Get("Accept")
	if header == "" {
		return true
	}

	for _, field := range strings.Split(header, ",") {
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
	defer body.Close()

	// TODO: size limit

	if err := json.NewDecoder(body).Decode(v); err != nil {
		writeBadRequest(w, r, err)
		return
	}

	ok = true
	return
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

func writeOptionsWebsocket(w http.ResponseWriter, r *http.Request, allow string) {
	w.Header().Set("Allow", allow)
	writeText(w, r, http.StatusOK, "WebSocket endpoint")
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
