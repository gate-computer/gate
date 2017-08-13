package webserver

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
	internal "github.com/tsavola/gate/internal/server"
	"github.com/tsavola/gate/server"
	api "github.com/tsavola/gate/webapi"
	"github.com/tsavola/wag/traps"
)

const (
	wasmRequestMaxSize = 16 * 1024 * 1024 // conservative

	accessControlMaxAge = "86400"
)

// NewHandler uses http.Request's context for resources whose lifetimes are
// tied to requests.  The context specified here is used for other resources,
// which are merely created by requests.
func NewHandler(ctx context.Context, pattern string, state *server.State) http.Handler {
	var (
		s      = &state.Internal
		prefix = strings.TrimRight(pattern, "/")
		mux    = http.NewServeMux()
	)

	{
		var (
			path          = prefix + "/load"
			allowMethods  = joinHeader(http.MethodPost, http.MethodOptions)
			allowHeaders  = joinHeader("Content-Type", api.HeaderProgramId, api.HeaderProgramSHA512)
			exposeHeaders = joinHeader(api.HeaderProgramId)
		)

		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			writeCORS(w, r, allowMethods, allowHeaders, exposeHeaders)

			switch r.Method {
			case http.MethodPost:
				switch getContentType(r) {
				case "application/wasm":
					handleLoadContent(w, r, s)

				case "":
					handleLoadId(w, r, s)

				default:
					writeUnsupportedMediaType(w)
				}

			case http.MethodOptions:
				writeOptions(w, allowMethods)

			default:
				writeMethodNotAllowed(w, allowMethods)
			}
		})
	}

	{
		var (
			path          = prefix + "/spawn"
			allowMethods  = joinHeader(http.MethodPost, http.MethodOptions)
			allowHeaders  = joinHeader("Content-Type", api.HeaderProgramId, api.HeaderProgramSHA512)
			exposeHeaders = joinHeader(api.HeaderInstanceId, api.HeaderProgramId)
		)

		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			writeCORS(w, r, allowMethods, allowHeaders, exposeHeaders)

			switch r.Method {
			case http.MethodPost:
				switch getContentType(r) {
				case "application/wasm":
					handleSpawnContent(ctx, w, r, s)

				case "":
					handleSpawnId(ctx, w, r, s)

				default:
					writeUnsupportedMediaType(w)
				}

			case http.MethodOptions:
				writeOptions(w, allowHeaders)

			default:
				writeMethodNotAllowed(w, allowHeaders)
			}
		})
	}

	{
		var (
			path             = prefix + "/run"
			allowMethods     = joinHeader(http.MethodGet, http.MethodPost, http.MethodOptions)
			allowMethodsCORS = joinHeader(http.MethodPost, http.MethodOptions) // exclude websocket
			allowHeaders     = joinHeader(api.HeaderProgramId, api.HeaderProgramSHA512)
			exposeHeaders    = joinHeader(api.HeaderInstanceId, api.HeaderProgramId)
		)

		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			writeCORS(w, r, allowMethodsCORS, allowHeaders, exposeHeaders)

			switch r.Method {
			case http.MethodGet:
				handleRunWebsocket(w, r, s)

			case http.MethodPost:
				handleRunPost(w, r, s)

			case http.MethodOptions:
				writeOptions(w, allowMethods)

			default:
				writeMethodNotAllowed(w, allowMethods)
			}
		})
	}

	{
		var (
			path             = prefix + "/communicate"
			allowMethods     = joinHeader(http.MethodGet, http.MethodPost, http.MethodOptions)
			allowMethodsCORS = joinHeader(http.MethodPost, http.MethodOptions) // exclude websocket
			allowHeaders     = joinHeader(api.HeaderInstanceId)
		)

		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			writeCORSWithoutExposeHeaders(w, r, allowMethodsCORS, allowHeaders)

			switch r.Method {
			case http.MethodGet:
				handleCommunicateWebsocket(w, r, s)

			case http.MethodPost:
				handleCommunicatePost(w, r, s)

			case http.MethodOptions:
				writeOptions(w, allowMethods)

			default:
				writeMethodNotAllowed(w, allowMethods)
			}
		})
	}

	{
		var (
			path          = prefix + "/wait"
			allowMethods  = joinHeader(http.MethodPost, http.MethodOptions)
			allowHeaders  = joinHeader(api.HeaderInstanceId)
			exposeHeaders = joinHeader(api.HeaderError, api.HeaderExitStatus, api.HeaderTrap, api.HeaderTrapId)
		)

		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			writeCORS(w, r, allowMethods, allowHeaders, exposeHeaders)

			switch r.Method {
			case http.MethodPost:
				switch getContentType(r) {
				case "":
					handleWait(w, r, s)

				default:
					writeUnsupportedMediaType(w)
				}

			case http.MethodOptions:
				writeOptions(w, allowMethods)

			default:
				writeMethodNotAllowed(w, allowMethods)
			}
		})
	}

	return mux
}

func handleLoadContent(w http.ResponseWriter, r *http.Request, s *internal.State) {
	progHexHash, ok := requireHeader(w, r, api.HeaderProgramSHA512)
	if !ok {
		return
	}

	var (
		progHash []byte
		progId   uint64
		valid    bool
		err      error
	)

	if progHash, err = hex.DecodeString(progHexHash); err == nil {
		body := decodeContent(w, r, s, wasmRequestMaxSize)
		if body == nil {
			return
		}

		// upload method closes body to check for decoding errors

		progId, progHash, valid, err = s.Upload(body, progHash)
		if err != nil {
			writeBadRequest(w, r, err) // TODO: don't leak sensitive information
			return
		}
	}
	if !valid {
		writeText(w, r, http.StatusBadRequest, "SHA-512 hash mismatch")
		return
	}

	w.Header().Set(api.HeaderProgramId, makeHexId(progId))
}

func handleLoadId(w http.ResponseWriter, r *http.Request, s *internal.State) {
	progHexId, ok := requireHeader(w, r, api.HeaderProgramId)
	if !ok {
		return
	}

	progHexHash, ok := requireHeader(w, r, api.HeaderProgramSHA512)
	if !ok {
		return
	}

	var (
		valid bool
		found bool
	)

	if progId, err := strconv.ParseUint(progHexId, 16, 64); err == nil {
		if progHash, err := hex.DecodeString(progHexHash); err == nil {
			valid, found = s.Check(progId, progHash)
		}
	}
	if !found {
		http.NotFound(w, r)
		return
	}
	if !valid {
		writeText(w, r, http.StatusForbidden, "SHA-512 hash mismatch")
		return
	}
}

func handleSpawnContent(ctx context.Context, w http.ResponseWriter, r *http.Request, s *internal.State) {
	progHexHash, ok := requireHeader(w, r, api.HeaderProgramSHA512)
	if !ok {
		return
	}

	in, out, originPipe := internal.NewPipe()

	ctx, cancel := context.WithCancel(ctx)
	defer func() {
		if cancel != nil {
			cancel()
		}
	}()

	var (
		inst     *internal.Instance
		instId   uint64
		progId   uint64
		progHash []byte
		valid    bool
		err      error
	)

	if progHash, err = hex.DecodeString(progHexHash); err == nil {
		body := decodeContent(w, r, s, wasmRequestMaxSize)
		if body == nil {
			return
		}

		// uploadAndInstantiate method closes body to check for decoding errors

		inst, instId, progId, progHash, valid, err = s.UploadAndInstantiate(r.Context(), body, progHash, originPipe, cancel)
		if err != nil {
			writeBadRequest(w, r, err) // TODO: don't leak sensitive information
			return
		}
	}
	if !valid {
		writeText(w, r, http.StatusForbidden, "SHA-512 hash mismatch")
		return
	}

	w.Header().Set(api.HeaderInstanceId, makeHexId(instId))
	w.Header().Set(api.HeaderProgramId, makeHexId(progId))

	go func(cancel context.CancelFunc) {
		defer cancel()
		defer out.Close()
		inst.Run(ctx, &s.Options, in, out)
	}(cancel)
	cancel = nil
}

func handleSpawnId(ctx context.Context, w http.ResponseWriter, r *http.Request, s *internal.State) {
	progHexId, ok := requireHeader(w, r, api.HeaderProgramId)
	if !ok {
		return
	}

	progHexHash, ok := requireHeader(w, r, api.HeaderProgramSHA512)
	if !ok {
		return
	}

	in, out, originPipe := internal.NewPipe()

	ctx, cancel := context.WithCancel(ctx)
	defer func() {
		if cancel != nil {
			cancel()
		}
	}()

	var (
		inst   *internal.Instance
		instId uint64
		valid  bool
		found  bool
	)

	if progId, err := strconv.ParseUint(progHexId, 16, 64); err == nil {
		if progHash, err := hex.DecodeString(progHexHash); err == nil {
			inst, instId, valid, found, err = s.Instantiate(r.Context(), progId, progHash, originPipe, cancel)
			if err != nil {
				writeBadRequest(w, r, err) // TODO: don't leak sensitive information
				return
			}
		}
	}
	if !found {
		http.NotFound(w, r)
		return
	}
	if !valid {
		writeText(w, r, http.StatusForbidden, "SHA-512 hash mismatch")
		return
	}

	w.Header().Set(api.HeaderInstanceId, makeHexId(instId))

	go func(cancel context.CancelFunc) {
		defer cancel()
		defer out.Close()
		inst.Run(ctx, &s.Options, in, out)
	}(cancel)
	cancel = nil
}

func handleRunWebsocket(w http.ResponseWriter, r *http.Request, s *internal.State) {
	u := websocket.Upgrader{
		CheckOrigin: func(*http.Request) bool { return true },
	}

	conn, err := u.Upgrade(w, r, nil)
	if err != nil {
		s.Log.Printf("%s error: %v", r.RemoteAddr, err)
		return
	}
	defer conn.Close()

	// TODO: size limit

	var run api.Run

	err = conn.ReadJSON(&run)
	if err != nil {
		// TODO
		s.Log.Printf("%s error: %v", r.RemoteAddr, err)
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	var (
		inst   *internal.Instance
		instId uint64
		valid  bool

		progHexId string
	)

	if run.ProgramId != "" {
		var (
			found bool
		)

		if progId, err := strconv.ParseUint(run.ProgramId, 16, 64); err == nil {
			if progHash, err := hex.DecodeString(run.ProgramSHA512); err == nil {
				inst, instId, valid, found, err = s.Instantiate(ctx, progId, progHash, nil, cancel)
				if err != nil {
					// TODO
					s.Log.Printf("%s error: %v", r.RemoteAddr, err)
					return
				}
			}
		}
		if !found {
			// TODO
			s.Log.Printf("%s error: not found", r.RemoteAddr)
			return
		}
	} else {
		frameType, frame, err := conn.NextReader()
		if err != nil {
			s.Log.Printf("%s error: %v", r.RemoteAddr, err)
			return
		}
		if frameType != websocket.BinaryMessage {
			conn.WriteMessage(websocket.CloseMessage, websocketUnsupportedData)
			return
		}

		// TODO: size limit

		var (
			progId   uint64
			progHash []byte
		)

		if progHash, err = hex.DecodeString(run.ProgramSHA512); err == nil {
			inst, instId, progId, progHash, valid, err = s.UploadAndInstantiate(ctx, ioutil.NopCloser(frame), progHash, nil, cancel)
			if err != nil {
				// TODO
				s.Log.Printf("%s error: %v", r.RemoteAddr, err)
				return
			}
		}

		progHexId = makeHexId(progId)
	}
	if !valid {
		// TODO
		s.Log.Printf("%s error: invalid hash", r.RemoteAddr)
		return
	}

	err = conn.WriteJSON(&api.Running{
		InstanceId: makeHexId(instId),
		ProgramId:  progHexId,
	})

	inst.Run(ctx, &s.Options, newWebsocketReader(conn), websocketWriter{conn})

	if err != nil {
		return
	}

	closeMsg := websocketNormalClosure

	if result, ok := s.WaitInstance(inst, instId); ok {
		if result != nil {
			var doc api.Result

			if result.Trap == traps.Exit {
				doc.ExitStatus = &result.Status
			} else {
				doc.TrapId = int(result.Trap)
				doc.Trap = result.Trap.String()
			}

			err = conn.WriteJSON(&doc)
			if err != nil {
				return
			}
		} else {
			closeMsg = websocketInternalServerErr
		}
	}

	conn.WriteMessage(websocket.CloseMessage, closeMsg)
}

func handleRunPost(w http.ResponseWriter, r *http.Request, s *internal.State) {
	progHexId, ok := requireHeader(w, r, api.HeaderProgramId)
	if !ok {
		return
	}

	progHexHash, ok := requireHeader(w, r, api.HeaderProgramSHA512)
	if !ok {
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	var (
		inst   *internal.Instance
		instId uint64
		valid  bool
		found  bool
	)

	if progId, err := strconv.ParseUint(progHexId, 16, 64); err == nil {
		if progHash, err := hex.DecodeString(progHexHash); err == nil {
			inst, instId, valid, found, err = s.Instantiate(ctx, progId, progHash, nil, cancel)
			if err != nil {
				writeBadRequest(w, r, err) // TODO: don't leak sensitive information
				return
			}
		}
	}
	if !found {
		http.NotFound(w, r)
		return
	}
	if !valid {
		writeText(w, r, http.StatusForbidden, "SHA-512 hash mismatch")
		return
	}

	w.Header().Set(api.HeaderInstanceId, makeHexId(instId))

	inst.Run(ctx, &s.Options, r.Body, w)

	if result, ok := s.WaitInstance(inst, instId); ok {
		if result != nil {
			setResultHeader(w, http.TrailerPrefix, result)
		} else {
			w.Header().Set(http.TrailerPrefix+api.HeaderError, "internal server error")
		}
	}
}

func handleCommunicateWebsocket(w http.ResponseWriter, r *http.Request, s *internal.State) {
	u := websocket.Upgrader{
		CheckOrigin: func(*http.Request) bool { return true },
	}

	conn, err := u.Upgrade(w, r, nil)
	if err != nil {
		s.Log.Printf("%s error: %v", r.RemoteAddr, err)
		return
	}
	defer conn.Close()

	// TODO: size limit

	var communicate api.Communicate

	err = conn.ReadJSON(&communicate)
	if err != nil {
		// TODO
		return
	}

	var (
		originPipe *internal.Pipe
		found      bool
	)

	if instId, err := strconv.ParseUint(communicate.InstanceId, 16, 64); err == nil {
		originPipe, found = s.AttachOrigin(instId)
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

	originPipe.IO(newWebsocketReader(conn), websocketWriter{conn})

	conn.WriteMessage(websocket.CloseMessage, websocketNormalClosure)
}

func handleCommunicatePost(w http.ResponseWriter, r *http.Request, s *internal.State) {
	instHexId, ok := requireHeader(w, r, api.HeaderInstanceId)
	if !ok {
		return
	}

	body := decodeUnlimitedContent(w, r, s)
	if body == nil {
		return
	}
	defer body.Close()

	var (
		originPipe *internal.Pipe
		found      bool
	)

	if instId, err := strconv.ParseUint(instHexId, 16, 64); err == nil {
		originPipe, found = s.AttachOrigin(instId)
	}
	if !found {
		http.NotFound(w, r)
		return
	}
	if originPipe == nil {
		writeText(w, r, http.StatusConflict, "Already communicating")
		return
	}

	originPipe.IO(body, w)
}

func handleWait(w http.ResponseWriter, r *http.Request, s *internal.State) {
	instHexId, ok := requireHeader(w, r, api.HeaderInstanceId)
	if !ok {
		return
	}

	var (
		result *internal.Result
		found  bool
	)

	if instId, err := strconv.ParseUint(instHexId, 16, 64); err == nil {
		result, found = s.Wait(instId)
	}
	if !found {
		http.NotFound(w, r)
		return
	}
	if result == nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	setResultHeader(w, "", result)
}

func setResultHeader(w http.ResponseWriter, prefix string, result *internal.Result) {
	if result.Trap == traps.Exit {
		w.Header().Set(prefix+api.HeaderExitStatus, strconv.Itoa(result.Status))
	} else {
		w.Header().Set(prefix+api.HeaderTrapId, strconv.Itoa(int(result.Trap)))
		w.Header().Set(prefix+api.HeaderTrap, result.Trap.String())
	}
}

func joinHeader(fields ...string) string {
	return strings.Join(fields, ", ")
}

func acceptsText(r *http.Request) bool {
	fields := r.Header["Accept"]
	if len(fields) == 0 {
		return true
	}

	for _, field := range fields {
		tokens := strings.SplitN(field, ";", 2)
		mediaType := strings.TrimSpace(tokens[0])

		switch mediaType {
		case "*/*", "text/plain", "text/*":
			return true
		}
	}

	return false
}

func getContentType(r *http.Request) (value string) {
	if fields := r.Header["Content-Type"]; len(fields) > 0 {
		tokens := strings.SplitN(fields[0], ";", 2)
		value = strings.TrimSpace(tokens[0])
	}
	return
}

func requireHeader(w http.ResponseWriter, r *http.Request, canonicalKey string) (value string, ok bool) {
	fields := r.Header[canonicalKey]

	switch len(fields) {
	case 1:
		return fields[0], true

	case 0:
		writeText(w, r, http.StatusBadRequest, canonicalKey, " header is missing")
		return "", false

	default:
		writeText(w, r, http.StatusBadRequest, canonicalKey, " header is invalid")
		return "", false
	}
}

func decodeUnlimitedContent(w http.ResponseWriter, r *http.Request, s *internal.State) io.ReadCloser {
	// TODO: support nested encodings

	var encoding string

	if fields := r.Header["Content-Encoding"]; len(fields) > 0 {
		encoding = fields[0]
	}

	switch encoding { // non-standard for request
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

func decodeContent(w http.ResponseWriter, r *http.Request, s *internal.State, limit int) (cr *contentReader) {
	if r.ContentLength < 0 {
		w.WriteHeader(http.StatusLengthRequired)
		return
	}
	if r.ContentLength > int64(limit) {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		return
	}

	if c := decodeUnlimitedContent(w, r, s); c != nil {
		cr = newContentReader(c, limit)
	}
	return
}

func writeCORSWithoutExposeHeaders(w http.ResponseWriter, r *http.Request, allowMethods, allowHeaders string) (origin bool) {
	_, origin = r.Header["Origin"]
	if origin {
		w.Header().Set("Access-Control-Allow-Headers", allowHeaders)
		w.Header().Set("Access-Control-Allow-Methods", allowMethods)
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Max-Age", accessControlMaxAge)
	}
	return
}

func writeCORS(w http.ResponseWriter, r *http.Request, allowMethods, allowHeaders, exposeHeaders string) {
	if writeCORSWithoutExposeHeaders(w, r, allowMethods, allowHeaders) {
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

func writeMethodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	w.WriteHeader(http.StatusMethodNotAllowed)
}

func writeUnsupportedMediaType(w http.ResponseWriter) {
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

func makeHexId(id uint64) string {
	return fmt.Sprintf("%016x", id)
}
