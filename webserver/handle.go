// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webserver

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/tsavola/gate/internal/publicerror"
	internal "github.com/tsavola/gate/internal/server"
	"github.com/tsavola/gate/server"
	"github.com/tsavola/gate/server/detail"
	"github.com/tsavola/gate/server/event"
	api "github.com/tsavola/gate/webapi"
	"github.com/tsavola/wag/traps"
)

const (
	accessControlMaxAge = "86400"
)

var (
	errContentHashMismatch  = errors.New("Program SHA-384 hash does not match content")
	errContentNotEmpty      = errors.New("Request content must be empty")
	errEncodingNotSupported = errors.New("The only supported Content-Encoding is gzip")
	errLengthRequired       = errors.New(http.StatusText(http.StatusLengthRequired))
	errMethodNotAllowed     = errors.New(http.StatusText(http.StatusMethodNotAllowed))
	errProgramTooLarge      = errors.New("Program content is too large")
	errUnsupportedMediaType = errors.New(http.StatusText(http.StatusUnsupportedMediaType))
)

// NewHandler should be called with the same context that was passed to
// server.NewState(), or its subcontext.
func NewHandler(ctx context.Context, pattern string, state *server.State) http.Handler {
	var (
		s           = &state.Internal
		maxProgSize = s.MaxProgramSize
		mux         = http.NewServeMux()
	)

	handler := func(w http.ResponseWriter, r *http.Request) {
		ctx := server.WithClient(ctx, r.RemoteAddr)

		s.MonitorEvent(&event.ServerAccess{
			Context:  server.Context(ctx),
			Protocol: r.Proto,
			Method:   r.Method,
			Url:      r.URL.String(),
		}, nil)

		mux.ServeHTTP(w, r.WithContext(ctx))
	}

	{
		const call = "load"

		var (
			allowMethods  = joinHeader(http.MethodPost, http.MethodOptions)
			allowHeaders  = joinHeader("Content-Type", api.HeaderProgramSHA384, api.HeaderProgramId)
			exposeHeaders = joinHeader(api.HeaderProgramId)
		)

		mux.HandleFunc(path.Join(pattern, call), func(w http.ResponseWriter, r *http.Request) {
			ctx := server.WithCall(r.Context(), call)

			writeCORS(w, r, allowMethods, allowHeaders, exposeHeaders)

			switch r.Method {
			case http.MethodPost:
				switch getContentType(r) {
				case "application/wasm":
					ctx = server.WithCall(ctx, call+" content")
					handleLoadContent(ctx, w, r, s, maxProgSize)

				case "":
					ctx = server.WithCall(ctx, call+" id")
					handleLoadId(ctx, w, r, s)

				default:
					writeUnsupportedMediaType(ctx, w, r, s)
				}

			case http.MethodOptions:
				writeOptions(w, allowMethods)

			default:
				writeMethodNotAllowed(ctx, w, r, s, allowMethods)
			}
		})
	}

	{
		const call = "spawn"

		var (
			allowMethods  = joinHeader(http.MethodPost, http.MethodOptions)
			allowHeaders  = joinHeader("Content-Type", api.HeaderProgramSHA384, api.HeaderProgramId, api.HeaderInstanceArg)
			exposeHeaders = joinHeader(api.HeaderInstanceId, api.HeaderProgramId)
		)

		mux.HandleFunc(path.Join(pattern, call), func(w http.ResponseWriter, r *http.Request) {
			ctx := server.WithCall(r.Context(), call)

			writeCORS(w, r, allowMethods, allowHeaders, exposeHeaders)

			switch r.Method {
			case http.MethodPost:
				switch getContentType(r) {
				case "application/wasm":
					ctx = server.WithCall(ctx, call+" content")
					handleSpawnContent(ctx, w, r, s, maxProgSize)

				case "":
					ctx = server.WithCall(ctx, call+" id")
					handleSpawnId(ctx, w, r, s)

				default:
					writeUnsupportedMediaType(ctx, w, r, s)
				}

			case http.MethodOptions:
				writeOptions(w, allowHeaders)

			default:
				writeMethodNotAllowed(ctx, w, r, s, allowHeaders)
			}
		})
	}

	{
		const call = "run"

		var (
			allowMethods     = joinHeader(http.MethodGet, http.MethodPost, http.MethodOptions)
			allowMethodsCORS = joinHeader(http.MethodPost, http.MethodOptions) // exclude websocket
			allowHeaders     = joinHeader(api.HeaderProgramSHA384, api.HeaderProgramId, api.HeaderInstanceArg)
			exposeHeaders    = joinHeader(api.HeaderInstanceId, api.HeaderProgramId)
		)

		mux.HandleFunc(path.Join(pattern, call), func(w http.ResponseWriter, r *http.Request) {
			ctx := server.WithCall(r.Context(), call)

			writeCORS(w, r, allowMethodsCORS, allowHeaders, exposeHeaders)

			switch r.Method {
			case http.MethodGet:
				ctx = server.WithCall(ctx, call+" socket")
				handleRunSocket(ctx, w, r, s)

			case http.MethodPost:
				ctx = server.WithCall(ctx, call+" post")
				handleRunPost(ctx, w, r, s)

			case http.MethodOptions:
				writeOptions(w, allowMethods)

			default:
				writeMethodNotAllowed(ctx, w, r, s, allowMethods)
			}
		})
	}

	{
		const call = "io"

		var (
			allowMethods     = joinHeader(http.MethodGet, http.MethodPost, http.MethodOptions)
			allowMethodsCORS = joinHeader(http.MethodPost, http.MethodOptions) // exclude websocket
			allowHeaders     = joinHeader(api.HeaderInstanceId)
		)

		mux.HandleFunc(path.Join(pattern, call), func(w http.ResponseWriter, r *http.Request) {
			ctx := server.WithCall(r.Context(), call)

			writeCORSWithoutExposeHeaders(w, r, allowMethodsCORS, allowHeaders)

			switch r.Method {
			case http.MethodGet:
				ctx = server.WithCall(ctx, call+" socket")
				handleIOSocket(ctx, w, r, s)

			case http.MethodPost:
				ctx = server.WithCall(ctx, call+" post")
				handleIOPost(ctx, w, r, s)

			case http.MethodOptions:
				writeOptions(w, allowMethods)

			default:
				writeMethodNotAllowed(ctx, w, r, s, allowMethods)
			}
		})
	}

	{
		const call = "wait"

		var (
			allowMethods  = joinHeader(http.MethodPost, http.MethodOptions)
			allowHeaders  = joinHeader(api.HeaderInstanceId)
			exposeHeaders = joinHeader(api.HeaderError, api.HeaderExitStatus, api.HeaderTrap, api.HeaderTrapId)
		)

		mux.HandleFunc(path.Join(pattern, call), func(w http.ResponseWriter, r *http.Request) {
			ctx := server.WithCall(r.Context(), call)

			writeCORS(w, r, allowMethods, allowHeaders, exposeHeaders)

			switch r.Method {
			case http.MethodPost:
				if r.ContentLength == 0 {
					handleWait(ctx, w, r, s)
				} else {
					writeContentNotEmpty(ctx, w, r, s)
				}

			case http.MethodOptions:
				writeOptions(w, allowMethods)

			default:
				writeMethodNotAllowed(ctx, w, r, s, allowMethods)
			}
		})
	}

	{
		var (
			allowMethods = joinHeader(http.MethodGet, http.MethodHead, http.MethodPost, http.MethodOptions)
		)

		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodOptions:
				handleNotFound(ctx, w, r, s)

			default:
				writeMethodNotAllowed(ctx, w, r, s, allowMethods)
			}
		})
	}

	return http.HandlerFunc(handler)
}

func handleLoadContent(ctx context.Context, w http.ResponseWriter, r *http.Request, s *internal.State, maxProgSize int) {
	progHash, ok := requireHeader(ctx, w, r, s, api.HeaderProgramSHA384)
	if !ok {
		return
	}

	body := decodeProgramContent(ctx, w, r, s, maxProgSize)
	if body == nil {
		return
	}

	// Upload method closes body to check for decoding errors

	progId, valid, err := s.Upload(ctx, body, progHash)
	if err != nil {
		writeError(ctx, w, r, s, "", 0, "", err)
		return
	}
	if !valid {
		writeProtocolError(ctx, w, r, s, http.StatusBadRequest, errContentHashMismatch)
		return
	}

	w.Header().Set(api.HeaderProgramId, progId)
}

func handleLoadId(ctx context.Context, w http.ResponseWriter, r *http.Request, s *internal.State) {
	progHash, ok := requireHeader(ctx, w, r, s, api.HeaderProgramSHA384)
	if !ok {
		return
	}

	progId, ok := requireHeader(ctx, w, r, s, api.HeaderProgramId)
	if !ok {
		return
	}

	valid, found := s.Check(ctx, progHash, progId)
	if !found {
		writeProgramNotFound(ctx, w, r, s, progId)
		return
	}
	if !valid {
		writeProgramHashMismatch(ctx, w, r, s, http.StatusForbidden, progId)
		return
	}
}

func handleSpawnContent(ctx context.Context, w http.ResponseWriter, r *http.Request, s *internal.State, maxProgSize int) {
	progHash, ok := requireHeader(ctx, w, r, s, api.HeaderProgramSHA384)
	if !ok {
		return
	}

	instArg, ok := parseInstanceArgHeader(ctx, w, r, s)
	if !ok {
		return
	}

	body := decodeProgramContent(ctx, w, r, s, maxProgSize)
	if body == nil {
		return
	}

	// uploadAndInstantiate method closes body to check for decoding errors

	in, out, originPipe := internal.NewPipe()

	inst, instId, progId, valid, err := s.UploadAndInstantiate(ctx, body, progHash, originPipe)
	if err != nil {
		writeInstantiationError(ctx, w, r, s, "", instArg, err)
		return
	}
	if !valid {
		writeProtocolError(ctx, w, r, s, http.StatusBadRequest, errContentHashMismatch)
		return
	}

	w.Header().Set(api.HeaderInstanceId, instId)
	w.Header().Set(api.HeaderProgramId, progId)

	go func() {
		defer out.Close()
		inst.Run(ctx, progId, instArg, instId, in, out, s)
	}()
}

func handleSpawnId(ctx context.Context, w http.ResponseWriter, r *http.Request, s *internal.State) {
	progHash, ok := requireHeader(ctx, w, r, s, api.HeaderProgramSHA384)
	if !ok {
		return
	}

	progId, ok := requireHeader(ctx, w, r, s, api.HeaderProgramId)
	if !ok {
		return
	}

	instArg, ok := parseInstanceArgHeader(ctx, w, r, s)
	if !ok {
		return
	}

	in, out, originPipe := internal.NewPipe()

	inst, instId, valid, found, err := s.Instantiate(ctx, progHash, progId, originPipe)
	if err != nil {
		writeInstantiationError(ctx, w, r, s, progId, instArg, err)
		return
	}
	if !found {
		writeProgramNotFound(ctx, w, r, s, progId)
		return
	}
	if !valid {
		writeProgramHashMismatch(ctx, w, r, s, http.StatusForbidden, progId)
		return
	}

	w.Header().Set(api.HeaderInstanceId, instId)

	go func() {
		defer out.Close()
		inst.Run(ctx, progId, instArg, instId, in, out, s)
	}()
}

func handleRunSocket(ctx context.Context, w http.ResponseWriter, r *http.Request, s *internal.State) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	u := websocket.Upgrader{
		CheckOrigin: allowAnyOrigin,
	}

	conn, err := u.Upgrade(w, r, nil)
	if err != nil {
		reportProtocolError(ctx, s, err)
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
		if _, ok := err.(net.Error); ok {
			reportNetworkError(ctx, s, err)
		} else {
			reportProtocolError(ctx, s, err)
		}
		return
	}

	var (
		inst   *internal.Instance
		instId string
		valid  bool
	)

	if run.ProgramId != "" {
		var (
			found bool
		)

		inst, instId, valid, found, err = s.Instantiate(ctx, run.ProgramSHA384, run.ProgramId, nil)
		if err != nil {
			writeInstantiationError(ctx, w, r, s, run.ProgramId, run.InstanceArg, err)
			return
		}
		if !found {
			writeProgramNotFound(ctx, w, r, s, run.ProgramId)
			return
		}
		if !valid {
			writeProgramHashMismatch(ctx, w, r, s, http.StatusForbidden, run.ProgramId)
			return
		}
	} else {
		frameType, frame, err := conn.NextReader()
		if err != nil {
			reportNetworkError(ctx, s, err)
			return
		}
		if frameType != websocket.BinaryMessage {
			conn.WriteMessage(websocket.CloseMessage, websocketUnsupportedData)
			reportProtocolError(ctx, s, errWrongWebsocketMessageType)
			return
		}

		// TODO: size limit

		inst, instId, run.ProgramId, valid, err = s.UploadAndInstantiate(ctx, ioutil.NopCloser(frame), run.ProgramSHA384, nil)
		if err != nil {
			writeInstantiationError(ctx, w, r, s, "", run.InstanceArg, err)
			return
		}
		if !valid {
			writeProtocolError(ctx, w, r, s, http.StatusBadRequest, errContentHashMismatch)
			return
		}
	}

	err = conn.WriteJSON(&api.Running{
		InstanceId: instId,
		ProgramId:  run.ProgramId,
	})
	if err != nil {
		cancel()
	}

	inst.Run(ctx, run.ProgramId, run.InstanceArg, instId, newWebsocketReadCanceler(conn, cancel), websocketWriter{conn}, s)

	closeMsg := websocketNormalClosure

	if result, ok := s.WaitInstance(ctx, inst, instId); ok {
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
				reportNetworkError(ctx, s, err)
				return
			}
		} else {
			closeMsg = websocketInternalServerErr
		}
	}

	conn.WriteMessage(websocket.CloseMessage, closeMsg)
}

func handleRunPost(ctx context.Context, w http.ResponseWriter, r *http.Request, s *internal.State) {
	progHash, ok := requireHeader(ctx, w, r, s, api.HeaderProgramSHA384)
	if !ok {
		return
	}

	progId, ok := requireHeader(ctx, w, r, s, api.HeaderProgramId)
	if !ok {
		return
	}

	instArg, ok := parseInstanceArgHeader(ctx, w, r, s)
	if !ok {
		return
	}

	inst, instId, valid, found, err := s.Instantiate(ctx, progHash, progId, nil)
	if err != nil {
		writeInstantiationError(ctx, w, r, s, progId, instArg, err)
		return
	}
	if !found {
		writeProgramNotFound(ctx, w, r, s, progId)
		return
	}
	if !valid {
		writeProgramHashMismatch(ctx, w, r, s, http.StatusForbidden, progId)
		return
	}

	w.Header().Set(api.HeaderInstanceId, instId)

	inst.Run(ctx, progId, instArg, instId, r.Body, w, s)

	if result, ok := s.WaitInstance(ctx, inst, instId); ok {
		if result != nil {
			setResultHeader(w, http.TrailerPrefix, result)
		} else {
			w.Header().Set(http.TrailerPrefix+api.HeaderError, "internal server error")
		}
	}
}

func handleIOSocket(ctx context.Context, w http.ResponseWriter, r *http.Request, s *internal.State) {
	u := websocket.Upgrader{
		CheckOrigin: allowAnyOrigin,
	}

	conn, err := u.Upgrade(w, r, nil)
	if err != nil {
		reportProtocolError(ctx, s, err)
		return
	}
	defer conn.Close()

	// TODO: size limit

	var io api.IO

	err = conn.ReadJSON(&io)
	if err != nil {
		if _, ok := err.(net.Error); ok {
			reportNetworkError(ctx, s, err)
		} else {
			reportProtocolError(ctx, s, err)
		}
		return
	}

	originPipe, found := s.AttachOrigin(ctx, io.InstanceId)
	if !found {
		conn.WriteMessage(websocket.CloseMessage, websocketNormalClosure)
		reportInstanceNotFound(ctx, s, io.InstanceId)
		return
	}
	if originPipe == nil {
		conn.WriteMessage(websocket.CloseMessage, websocketIOAlreadyAttached)
		reportIOConflict(ctx, s, io.InstanceId)
		return
	}
	defer originPipe.DetachOrigin(ctx, io.InstanceId, s)

	err = conn.WriteJSON(api.IOState{})
	if err != nil {
		reportNetworkError(ctx, s, err)
		return
	}

	originPipe.IO(newWebsocketReader(conn), websocketWriter{conn})

	conn.WriteMessage(websocket.CloseMessage, websocketNormalClosure)
}

func handleIOPost(ctx context.Context, w http.ResponseWriter, r *http.Request, s *internal.State) {
	instId, ok := requireHeader(ctx, w, r, s, api.HeaderInstanceId)
	if !ok {
		return
	}

	body := decodeUnlimitedContent(ctx, w, r, s)
	if body == nil {
		return
	}
	defer body.Close()

	originPipe, found := s.AttachOrigin(ctx, instId)
	if !found {
		writeInstanceNotFound(ctx, w, r, s, instId)
		return
	}
	if originPipe == nil {
		writeIOConflict(ctx, w, r, s, instId)
		return
	}
	defer originPipe.DetachOrigin(ctx, instId, s)

	originPipe.IO(body, w)
}

func handleWait(ctx context.Context, w http.ResponseWriter, r *http.Request, s *internal.State) {
	instId, ok := requireHeader(ctx, w, r, s, api.HeaderInstanceId)
	if !ok {
		return
	}

	result, found := s.Wait(ctx, instId)
	if !found {
		writeInstanceNotFound(ctx, w, r, s, instId)
		return
	}
	if result == nil {
		writeHeader(w, r, http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError))
		// MonitorError already invoked by server.Instance.Run
		return
	}

	setResultHeader(w, "", result)
}

func handleNotFound(ctx context.Context, w http.ResponseWriter, r *http.Request, s *internal.State) {
	writeHeader(w, r, http.StatusNotFound, http.StatusText(http.StatusNotFound))
	reportProtocolError(ctx, s, fmt.Errorf("Path not found: %q", r.URL.Path))
}

func allowAnyOrigin(*http.Request) bool {
	return true
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

func requireHeader(ctx context.Context, w http.ResponseWriter, r *http.Request, s *internal.State, canonicalKey string,
) (value string, ok bool) {
	fields := r.Header[canonicalKey]

	switch len(fields) {
	case 1:
		return fields[0], true

	case 0:
		writeProtocolError(ctx, w, r, s, http.StatusBadRequest, fmt.Errorf("%s header is missing", canonicalKey))
		return "", false

	default:
		writeProtocolError(ctx, w, r, s, http.StatusBadRequest, fmt.Errorf("%s header is invalid", canonicalKey))
		return "", false
	}
}

func parseInstanceArgHeader(ctx context.Context, w http.ResponseWriter, r *http.Request, s *internal.State,
) (arg int32, ok bool) {
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

	writeProtocolError(ctx, w, r, s, http.StatusBadRequest, fmt.Errorf("%s header is invalid", api.HeaderInstanceArg))
	return
}

func decodeUnlimitedContent(ctx context.Context, w http.ResponseWriter, r *http.Request, s *internal.State,
) io.ReadCloser {
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
		} else {
			writeProtocolError(ctx, w, r, s, http.StatusBadRequest, err)
			return nil
		}

	default:
		w.Header().Set("Accept-Encoding", "gzip") // non-standard for response
		writeProtocolError(ctx, w, r, s, http.StatusBadRequest, errEncodingNotSupported)
		return nil
	}
}

func decodeProgramContent(ctx context.Context, w http.ResponseWriter, r *http.Request, s *internal.State, limit int,
) (cr *contentReader) {
	if r.ContentLength < 0 {
		writeProtocolError(ctx, w, r, s, http.StatusLengthRequired, errLengthRequired)
		return
	}
	if r.ContentLength > int64(limit) {
		writePayloadError(ctx, w, r, s, http.StatusRequestEntityTooLarge, errProgramTooLarge)
		return
	}

	if c := decodeUnlimitedContent(ctx, w, r, s); c != nil {
		cr = newContentReader(c, limit)
	}
	return
}

func writeCORSWithoutExposeHeaders(w http.ResponseWriter, r *http.Request, allowMethods, allowHeaders string,
) (origin bool) {
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

func writeMethodNotAllowed(ctx context.Context, w http.ResponseWriter, r *http.Request, s *internal.State, allow string) {
	w.Header().Set("Allow", allow)
	writeProtocolError(ctx, w, r, s, http.StatusMethodNotAllowed, errMethodNotAllowed)
}

func writeUnsupportedMediaType(ctx context.Context, w http.ResponseWriter, r *http.Request, s *internal.State) {
	writeProtocolError(ctx, w, r, s, http.StatusUnsupportedMediaType, errUnsupportedMediaType)
}

func writeContentNotEmpty(ctx context.Context, w http.ResponseWriter, r *http.Request, s *internal.State) {
	writeProtocolError(ctx, w, r, s, http.StatusRequestEntityTooLarge, errContentNotEmpty)
}

func writeProtocolError(ctx context.Context, w http.ResponseWriter, r *http.Request, s *internal.State, status int, err error) {
	writeHeader(w, r, status, err.Error())
	reportProtocolError(ctx, s, err)
}

func writePayloadError(ctx context.Context, w http.ResponseWriter, r *http.Request, s *internal.State, status int, err error) {
	writeHeader(w, r, status, err.Error())
	reportPayloadError(ctx, s, "", 0, "", err)
}

func writeProgramNotFound(ctx context.Context, w http.ResponseWriter, r *http.Request, s *internal.State, progId string) {
	writeHeader(w, r, http.StatusNotFound, "Program id is unknown")
	reportProgramNotFound(ctx, s, progId)
}

func writeProgramHashMismatch(ctx context.Context, w http.ResponseWriter, r *http.Request, s *internal.State, status int, progId string) {
	writeHeader(w, r, status, "Program SHA-384 hash mismatch")
	reportProgramHashMismatch(ctx, s, progId)
}

func writeInstanceNotFound(ctx context.Context, w http.ResponseWriter, r *http.Request, s *internal.State, instId string) {
	writeHeader(w, r, http.StatusNotFound, "Instance id is unknown")
	reportInstanceNotFound(ctx, s, instId)
}

func writeIOConflict(ctx context.Context, w http.ResponseWriter, r *http.Request, s *internal.State, instId string) {
	writeHeader(w, r, http.StatusConflict, "I/O origin already attached")
	reportIOConflict(ctx, s, instId)
}

func writeInstantiationError(ctx context.Context, w http.ResponseWriter, r *http.Request, s *internal.State, progId string, instArg int32, err error) {
	writeError(ctx, w, r, s, progId, instArg, "", err)
}

func writeError(ctx context.Context, w http.ResponseWriter, r *http.Request, s *internal.State, progId string, instArg int32, instId string, err error) {
	var (
		status    int
		text      string
		subsystem string
	)

	if puberr, ok := err.(publicerror.PublicError); ok {
		err = puberr.PrivateErr()
		text = puberr.PublicError()
		subsystem = puberr.Internal()
		if subsystem != "" {
			status = http.StatusInternalServerError
		} else {
			status = http.StatusBadRequest
		}
	} else {
		status = http.StatusInternalServerError
		text = http.StatusText(status)
	}

	writeHeader(w, r, status, text)

	if status >= 500 {
		s.MonitorError(&detail.Position{
			Context:     server.Context(ctx),
			ProgramId:   progId,
			InstanceArg: instArg,
			InstanceId:  instId,
			Subsystem:   subsystem,
		}, err)
	} else {
		reportPayloadError(ctx, s, progId, instArg, instId, err)
	}
}

func writeHeader(w http.ResponseWriter, r *http.Request, status int, text string) {
	if acceptsText(r) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(status)
		fmt.Fprintln(w, text)
	} else {
		w.WriteHeader(status)
	}
}

func reportNetworkError(ctx context.Context, s *internal.State, err error) {
	s.MonitorEvent(&event.FailNetwork{
		Context: server.Context(ctx),
	}, err)
}

func reportProtocolError(ctx context.Context, s *internal.State, err error) {
	s.MonitorEvent(&event.FailProtocol{
		Context: server.Context(ctx),
	}, err)
}

func reportPayloadError(ctx context.Context, s *internal.State, progId string, instArg int32, instId string, err error) {
	s.MonitorEvent(&event.FailRequest{
		Context:     server.Context(ctx),
		Type:        event.FailRequest_PAYLOAD_ERROR,
		ProgramId:   progId,
		InstanceArg: instArg,
		InstanceId:  instId,
	}, err)
}

func reportProgramNotFound(ctx context.Context, s *internal.State, progId string) {
	s.MonitorEvent(&event.FailRequest{
		Context:   server.Context(ctx),
		Type:      event.FailRequest_PROGRAM_NOT_FOUND,
		ProgramId: progId,
	}, nil)
}

func reportProgramHashMismatch(ctx context.Context, s *internal.State, progId string) {
	s.MonitorEvent(&event.FailRequest{
		Context:   server.Context(ctx),
		Type:      event.FailRequest_PROGRAM_HASH_MISMATCH,
		ProgramId: progId,
	}, nil)
}

func reportInstanceNotFound(ctx context.Context, s *internal.State, instId string) {
	s.MonitorEvent(&event.FailRequest{
		Context:    server.Context(ctx),
		Type:       event.FailRequest_INSTANCE_NOT_FOUND,
		InstanceId: instId,
	}, nil)
}

func reportIOConflict(ctx context.Context, s *internal.State, instId string) {
	s.MonitorEvent(&event.FailRequest{
		Context:    server.Context(ctx),
		Type:       event.FailRequest_IO_CONFLICT,
		InstanceId: instId,
	}, nil)
}
