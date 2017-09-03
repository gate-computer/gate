// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

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
	accessControlMaxAge = "86400"
)

// NewHandler should be called with the same context that was passed to
// server.NewState(), or its subcontext.
func NewHandler(ctx context.Context, pattern string, state *server.State, conf *Config) http.Handler {
	maxProgramSize := DefaultMaxProgramSize
	if conf != nil && conf.MaxProgramSize != 0 {
		maxProgramSize = conf.MaxProgramSize
	}

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
					handleLoadContent(w, r, s, maxProgramSize)

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
			allowHeaders  = joinHeader("Content-Type", api.HeaderProgramId, api.HeaderProgramSHA512, api.HeaderInstanceArg)
			exposeHeaders = joinHeader(api.HeaderInstanceId, api.HeaderProgramId)
		)

		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			writeCORS(w, r, allowMethods, allowHeaders, exposeHeaders)

			switch r.Method {
			case http.MethodPost:
				switch getContentType(r) {
				case "application/wasm":
					handleSpawnContent(ctx, w, r, s, maxProgramSize)

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
			allowHeaders     = joinHeader(api.HeaderProgramId, api.HeaderProgramSHA512, api.HeaderInstanceArg)
			exposeHeaders    = joinHeader(api.HeaderInstanceId, api.HeaderProgramId)
		)

		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			writeCORS(w, r, allowMethodsCORS, allowHeaders, exposeHeaders)

			switch r.Method {
			case http.MethodGet:
				handleRunWebsocket(ctx, w, r, s)

			case http.MethodPost:
				handleRunPost(ctx, w, r, s)

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

func handleLoadContent(w http.ResponseWriter, r *http.Request, s *internal.State, maxProgramSize int) {
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
		body := decodeContent(w, r, s, maxProgramSize)
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

	w.Header().Set(api.HeaderProgramId, internal.FormatId(progId))
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

	if progId, ok := internal.ParseId(progHexId); ok {
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

func handleSpawnContent(ctx context.Context, w http.ResponseWriter, r *http.Request, s *internal.State, maxProgramSize int) {
	r = r.WithContext(ctx)

	progHexHash, ok := requireHeader(w, r, api.HeaderProgramSHA512)
	if !ok {
		return
	}

	instArg, ok := parseInstanceArgHeader(w, r)
	if !ok {
		return
	}

	in, out, originPipe := internal.NewPipe()

	var (
		inst     *internal.Instance
		instId   uint64
		progId   uint64
		progHash []byte
		valid    bool
		err      error
	)

	if progHash, err = hex.DecodeString(progHexHash); err == nil {
		body := decodeContent(w, r, s, maxProgramSize)
		if body == nil {
			return
		}

		// uploadAndInstantiate method closes body to check for decoding errors

		inst, instId, progId, progHash, valid, err = s.UploadAndInstantiate(r.Context(), body, progHash, originPipe)
		if err != nil {
			writeBadRequest(w, r, err) // TODO: don't leak sensitive information
			return
		}
	}
	if !valid {
		writeText(w, r, http.StatusForbidden, "SHA-512 hash mismatch")
		return
	}

	w.Header().Set(api.HeaderInstanceId, internal.FormatId(instId))
	w.Header().Set(api.HeaderProgramId, internal.FormatId(progId))

	go func() {
		defer out.Close()
		inst.Run(ctx, s, instArg, in, out)
	}()
}

func handleSpawnId(ctx context.Context, w http.ResponseWriter, r *http.Request, s *internal.State) {
	r = r.WithContext(ctx)

	progHexId, ok := requireHeader(w, r, api.HeaderProgramId)
	if !ok {
		return
	}

	progHexHash, ok := requireHeader(w, r, api.HeaderProgramSHA512)
	if !ok {
		return
	}

	instArg, ok := parseInstanceArgHeader(w, r)
	if !ok {
		return
	}

	in, out, originPipe := internal.NewPipe()

	var (
		inst   *internal.Instance
		instId uint64
		valid  bool
		found  bool
	)

	if progId, ok := internal.ParseId(progHexId); ok {
		if progHash, err := hex.DecodeString(progHexHash); err == nil {
			inst, instId, valid, found, err = s.Instantiate(r.Context(), progId, progHash, originPipe)
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

	w.Header().Set(api.HeaderInstanceId, internal.FormatId(instId))

	go func() {
		defer out.Close()
		inst.Run(ctx, s, instArg, in, out)
	}()
}

func handleRunWebsocket(ctx context.Context, w http.ResponseWriter, r *http.Request, s *internal.State) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	u := websocket.Upgrader{
		CheckOrigin: func(*http.Request) bool { return true },
	}

	conn, err := u.Upgrade(w, r, nil)
	if err != nil {
		s.InfoLog.Printf("%s: run: %v", r.RemoteAddr, err)
		return
	}
	defer conn.Close()

	handleClose := conn.CloseHandler()
	conn.SetCloseHandler(func(code int, text string) error {
		cancel()
		return handleClose(code, text)
	})

	// TODO: size limit

	var run api.Run

	err = conn.ReadJSON(&run)
	if err != nil {
		// TODO
		s.InfoLog.Printf("%s: run: %v", r.RemoteAddr, err)
		return
	}

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

		if progId, ok := internal.ParseId(run.ProgramId); ok {
			if progHash, err := hex.DecodeString(run.ProgramSHA512); err == nil {
				inst, instId, valid, found, err = s.Instantiate(ctx, progId, progHash, nil)
				if err != nil {
					// TODO
					s.InfoLog.Printf("%s: run: %v", r.RemoteAddr, err)
					return
				}
			}
		}
		if !found {
			// TODO
			s.InfoLog.Printf("%s: run: program not found", r.RemoteAddr)
			return
		}
	} else {
		frameType, frame, err := conn.NextReader()
		if err != nil {
			s.InfoLog.Printf("%s: run: %v", r.RemoteAddr, err)
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
			inst, instId, progId, progHash, valid, err = s.UploadAndInstantiate(ctx, ioutil.NopCloser(frame), progHash, nil)
			if err != nil {
				// TODO
				s.InfoLog.Printf("%s: run: %v", r.RemoteAddr, err)
				return
			}
		}

		progHexId = internal.FormatId(progId)
	}
	if !valid {
		// TODO
		s.InfoLog.Printf("%s: run: invalid hash", r.RemoteAddr)
		return
	}

	err = conn.WriteJSON(&api.Running{
		InstanceId: internal.FormatId(instId),
		ProgramId:  progHexId,
	})
	if err != nil {
		cancel()
	}

	inst.Run(ctx, s, run.InstanceArg, newWebsocketReadCanceler(conn, cancel), websocketWriter{conn})

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

func handleRunPost(ctx context.Context, w http.ResponseWriter, r *http.Request, s *internal.State) {
	r = r.WithContext(ctx)

	progHexId, ok := requireHeader(w, r, api.HeaderProgramId)
	if !ok {
		return
	}

	progHexHash, ok := requireHeader(w, r, api.HeaderProgramSHA512)
	if !ok {
		return
	}

	instArg, ok := parseInstanceArgHeader(w, r)
	if !ok {
		return
	}

	var (
		inst   *internal.Instance
		instId uint64
		valid  bool
		found  bool
	)

	if progId, ok := internal.ParseId(progHexId); ok {
		if progHash, err := hex.DecodeString(progHexHash); err == nil {
			inst, instId, valid, found, err = s.Instantiate(r.Context(), progId, progHash, nil)
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

	w.Header().Set(api.HeaderInstanceId, internal.FormatId(instId))

	inst.Run(r.Context(), s, instArg, r.Body, w)

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
		s.InfoLog.Printf("%s: communicate: %v", r.RemoteAddr, err)
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

	if instId, ok := internal.ParseId(communicate.InstanceId); ok {
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

	if instId, ok := internal.ParseId(instHexId); ok {
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

	if instId, ok := internal.ParseId(instHexId); ok {
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

func parseInstanceArgHeader(w http.ResponseWriter, r *http.Request) (arg int32, ok bool) {
	fields := r.Header[api.HeaderInstanceArg]

	switch len(fields) {
	case 0:
		ok = true
		return

	case 1:
		if i, err := strconv.ParseInt(fields[0], 10, 32); err == nil {
			arg = int32(i)
			ok = true
			return
		}
	}

	writeText(w, r, http.StatusBadRequest, api.HeaderInstanceArg, " header is invalid")
	return
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

		s.InfoLog.Printf("%s: %v", r.RemoteAddr, err)

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
