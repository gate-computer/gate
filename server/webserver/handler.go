// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webserver

import (
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/tsavola/gate/internal/serverapi"
	"github.com/tsavola/gate/server"
	"github.com/tsavola/gate/server/detail"
	"github.com/tsavola/gate/server/event"
	"github.com/tsavola/gate/webapi"
)

const maxWebsocketRequestSize = 4096

var statusInternalServerErrorJSON = string(mustMarshalJSON(&webapi.Status{
	Error: "internal server error",
}))

type errorWriter interface {
	SetHeader(key, value string)
	WriteError(status int, text string)
}

type instanceMethod func(s *server.Server, ctx context.Context, pri *server.PrincipalKey, instance string) (server.Status, error)

type webserver struct {
	Config

	identity       string // JWT audience.
	pathModuleRefs string

	runBackgroundInstance func(*server.Instance)
}

// NewHandler should be called with the same context that was passed to
// server.New(), or its subcontext.
func NewHandler(ctx context.Context, pattern string, config *Config) http.Handler {
	s := &webserver{
		Config: *config,
	}

	s.runBackgroundInstance = func(inst *server.Instance) {
		inst.Run(ctx, s.Server)
	}

	if s.Authority == "" {
		s.Authority = strings.SplitN(pattern, "/", 2)[0]
	}
	if s.NewRequestID == nil {
		s.NewRequestID = defaultNewRequestID
	}
	if !s.Configured() {
		panic("incomplete webserver configuration")
	}

	p := strings.TrimRight(pattern, "/")                                // host/path
	pattern = p + "/"                                                   // host/path/
	patternAPI := p + webapi.Path                                       // host/path/api
	patternAPIDir := patternAPI + "/"                                   // host/path/api/
	patternModule := p + webapi.PathModule                              // host/path/api/module
	patternModules := p + webapi.PathModules                            // host/path/api/module/
	patternModuleRef := p + webapi.PathModules + webapi.ModuleRefSource // host/path/api/module/hash
	patternModuleRefs := p + webapi.PathModuleRefs                      // host/path/api/module/hash/
	patternInstances := p + webapi.PathInstances                        // host/path/api/instance/
	patternInstance := patternInstances[:len(patternInstances)-1]       // host/path/api/instance

	p = strings.TrimRight(strings.SplitN(pattern, "/", 2)[1], "/")   // /path
	pathAPI := p + webapi.Path                                       // /path/api
	pathAPIDir := pathAPI + "/"                                      // /path/api/
	pathModule := p + webapi.PathModule                              // /path/api/module
	pathModules := p + webapi.PathModules                            // /path/api/module/
	pathModuleRef := p + webapi.PathModules + webapi.ModuleRefSource // /path/api/module/hash
	s.pathModuleRefs = p + webapi.PathModuleRefs                     // /path/api/module/hash/
	pathInstances := p + webapi.PathInstances                        // /path/api/instance/
	pathInstance := pathInstances[:len(pathInstances)-1]             // /path/api/instance

	s.identity = "https://" + s.Authority + p + webapi.Path // https://authority/path/api

	mux := http.NewServeMux()
	mux.HandleFunc(pattern, newRootHandler(s, pattern))
	mux.HandleFunc(patternAPI, newStaticHandler(s, pathAPI, s.Server.Info))
	mux.HandleFunc(patternAPIDir, newOpaqueHandler(s, pathAPIDir))
	mux.HandleFunc(patternModule, newOpaqueHandler(s, pathModule))
	mux.HandleFunc(patternInstance, newOpaqueHandler(s, pathInstance))
	mux.HandleFunc(patternInstances, newInstanceHandler(s, pathInstances))
	mux.HandleFunc(patternModuleRef, newOpaqueHandler(s, pathModuleRef))
	mux.HandleFunc(patternModuleRefs, newModuleRefHandler(ctx, s))

	moduleSources := []string{webapi.ModuleRefSource}

	for relURI, source := range s.ModuleSources {
		patternSource := patternModule + relURI // host/path/api/module/source
		patternSourceDir := patternSource + "/" // host/path/api/module/source/

		pathSource := pathModule + relURI // /path/api/module/source
		pathSourceDir := pathSource + "/" // /path/api/module/source/

		mux.HandleFunc(patternSource, newOpaqueHandler(s, pathSource))
		mux.HandleFunc(patternSourceDir, newModuleSourceHandler(ctx, s, pathModule, pathSourceDir, source))

		moduleSources = append(moduleSources, strings.TrimLeft(relURI, "/"))
	}

	sort.Strings(moduleSources)
	mux.HandleFunc(patternModules, newStaticHandler(s, pathModules, moduleSources))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := server.ContextWithRequestAddr(ctx, s.NewRequestID(r), r.RemoteAddr)

		s.Server.Monitor(&event.IfaceAccess{
			Ctx: server.Context(ctx, nil),
		}, nil)

		defer func() {
			if x := recover(); x != nil {
				panic(x)
			}
		}()

		mux.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Path handlers.  Route methods and set up CORS.

func newOpaqueHandler(s *webserver, path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			respondPathNotFound(w, r, s)
			return
		}

		methods := "OPTIONS"
		setAccessControl(w, r, methods)

		switch r.Method {
		case "OPTIONS":
			setOptions(w, methods)

		default:
			respondMethodNotAllowed(w, r, s, methods)
		}
	}
}

func newRootHandler(s *webserver, path string) http.HandlerFunc {
	var (
		apiVersions   = []string{strings.TrimLeft(webapi.Path, "/")}
		content       = mustMarshalJSON(apiVersions)
		contentLength = strconv.Itoa(len(content))
	)

	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			respondPathNotFound(w, r, s)
			return
		}

		methods := "GET, HEAD, OPTIONS"
		setAccessControl(w, r, methods)

		switch r.Method {
		case "GET", "HEAD":
			handleGetStatic(w, r, s, content, contentLength)

		case "OPTIONS":
			setOptions(w, methods)

		default:
			respondMethodNotAllowed(w, r, s, methods)
		}
	}
}

func newStaticHandler(s *webserver, path string, data interface{}) http.HandlerFunc {
	var (
		content       = mustMarshalJSON(data)
		contentLength = strconv.Itoa(len(content))
	)

	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			respondPathNotFound(w, r, s)
			return
		}

		methods := "GET, HEAD, OPTIONS"
		setAccessControl(w, r, methods)

		switch r.Method {
		case "GET", "HEAD":
			handleGetStatic(w, r, s, content, contentLength)

		case "OPTIONS":
			setOptions(w, methods)

		default:
			respondMethodNotAllowed(w, r, s, methods)
		}
	}
}

func newModuleRefHandler(ctx context.Context, s *webserver) http.HandlerFunc {
	var (
		headersList = join(webapi.HeaderAuthorization)
		headersRef  = join(webapi.HeaderAuthorization, webapi.HeaderContentType)
		exposed     = join(webapi.HeaderLocation, webapi.HeaderInstance, webapi.HeaderStatus)
	)

	return func(w http.ResponseWriter, r *http.Request) {
		if len(r.URL.Path) == len(s.pathModuleRefs) {
			// Module directory listing

			methods := "GET, HEAD, OPTIONS"
			setAccessControlAllowHeaders(w, r, methods, headersList)

			switch r.Method {
			case "GET", "HEAD":
				handleGetModuleRefs(w, r, s)

			case "OPTIONS":
				setOptions(w, methods)

			default:
				respondMethodNotAllowed(w, r, s, methods)
			}
		} else {
			// Module operations
			module := r.URL.Path[len(s.pathModuleRefs):]

			methods := "GET, HEAD, OPTIONS, POST, PUT"
			setAccessControlAllowExposeHeaders(w, r, methods, headersRef, exposed)

			switch r.Method {
			case "GET", "HEAD":
				handleGetModuleRef(w, r, s, module)

			case "PUT":
				handlePutModuleRef(w, r, s, module)

			case "POST":
				handlePostModuleRef(w, r, s, module)

			case "OPTIONS":
				setOptions(w, methods)

			default:
				respondMethodNotAllowed(w, r, s, methods)
			}
		}
	}
}

func newModuleSourceHandler(ctx context.Context, s *webserver, sourceURIBase, sourcePath string, source server.Source) http.HandlerFunc {
	var (
		headers = join(webapi.HeaderAuthorization)
		exposed = join(webapi.HeaderLocation, webapi.HeaderInstance, webapi.HeaderStatus)
	)

	return func(w http.ResponseWriter, r *http.Request) {
		if len(r.URL.Path) == len(sourcePath) {
			// Module directory listing is not supported for sources.  The
			// directory clearly exists (it has modules in it), but doesn't
			// support any methods itself.

			methods := "OPTIONS"
			setAccessControl(w, r, methods)

			switch r.Method {
			case "OPTIONS":
				setOptions(w, methods)

			default:
				respondMethodNotAllowed(w, r, s, methods)
			}
		} else {
			// Module operations
			module := r.URL.Path[len(sourceURIBase):]

			// Get method is only for websocket; exclude it from CORS.
			methods := "OPTIONS, POST"
			setAccessControlAllowExposeHeaders(w, r, methods, headers, exposed)

			methods = "GET, OPTIONS, POST"

			switch r.Method {
			case "GET":
				handleGetModuleSource(w, r, s, source, module)

			case "POST":
				handlePostModuleSource(w, r, s, source, module)

			case "OPTIONS":
				setOptions(w, methods)

			default:
				respondMethodNotAllowed(w, r, s, methods)
			}
		}
	}
}

func newInstanceHandler(s *webserver, instancesPath string) http.HandlerFunc {
	var (
		headers = join(webapi.HeaderAuthorization)
		exposed = join(webapi.HeaderStatus)
	)

	return func(w http.ResponseWriter, r *http.Request) {
		if len(r.URL.Path) == len(instancesPath) {
			// Instance directory listing

			methods := "GET, HEAD, OPTIONS"
			setAccessControlAllowHeaders(w, r, methods, headers)

			switch r.Method {
			case "GET", "HEAD":
				handleGetInstances(w, r, s)

			case "OPTIONS":
				setOptions(w, methods)

			default:
				respondMethodNotAllowed(w, r, s, methods)
			}
		} else {
			// Instance operations
			instance := r.URL.Path[len(instancesPath):]

			// Get method is only for websocket; exclude it from CORS.
			methods := "OPTIONS, POST"
			setAccessControlAllowExposeHeaders(w, r, methods, headers, exposed)

			methods = "GET, OPTIONS, POST"

			switch r.Method {
			case "GET":
				handleGetInstance(w, r, s, instance)

			case "POST":
				handlePostInstance(w, r, s, instance)

			case "OPTIONS":
				setOptions(w, methods)

			default:
				respondMethodNotAllowed(w, r, s, methods)
			}
		}
	}
}

// Method handlers.  Parse query parameters and check content headers.

func handleGetStatic(w http.ResponseWriter, r *http.Request, s *webserver, content []byte, contentLength string) {
	mustNotHaveQuery(w, r, s)
	mustAcceptJSON(w, r, s)
	handleStatic(w, r, s, contentLength, content)
}

func handleGetModuleRefs(w http.ResponseWriter, r *http.Request, s *webserver) {
	mustNotHaveQuery(w, r, s)
	mustAcceptJSON(w, r, s)
	handleModules(w, r, s)
}

func handleGetModuleRef(w http.ResponseWriter, r *http.Request, s *webserver, key string) {
	if query := mustParseOptionalQuery(w, r, s); len(query) == 0 {
		mustAcceptWebAssembly(w, r, s)
		handleModuleGet(w, r, s, key)
	} else {
		switch mustPopParam(w, r, s, query, webapi.ParamAction) {
		case webapi.ActionCall:
			function := mustPopOptionalFunctionParam(w, r, s, query)
			debug := mustPopOptionalParam(w, r, s, query, webapi.ParamDebug)
			mustNotHaveParams(w, r, s, query)
			handleCallWebsocket(w, r, s, nil, key, function, debug)

		default:
			respondUnsupportedAction(w, r, s)
		}
	}
}

func handlePutModuleRef(w http.ResponseWriter, r *http.Request, s *webserver, key string) {
	mustNotHaveQuery(w, r, s)

	switch mustParseContentType(w, r, s) {
	case webapi.ContentTypeWebAssembly:
		mustHaveContentLength(w, r, s)
		handleModulePut(w, r, s, key)

	default:
		respondUnsupportedMediaType(w, r, s)
	}
}

func handlePostModuleRef(w http.ResponseWriter, r *http.Request, s *webserver, key string) {
	query := mustParseQuery(w, r, s)

	switch mustPopParam(w, r, s, query, webapi.ParamAction) {
	case webapi.ActionCall:
		function := mustPopOptionalFunctionParam(w, r, s, query)
		debug := mustPopOptionalParam(w, r, s, query, webapi.ParamDebug)
		mustNotHaveParams(w, r, s, query)

		switch mustParseContentType(w, r, s) {
		case webapi.ContentTypeWebAssembly:
			mustHaveContentLength(w, r, s)
			handleCallPost(w, r, s, server.OpCallRef, true, nil, key, function, debug)

		default:
			handleCallPost(w, r, s, server.OpCallRef, false, nil, key, function, debug)
		}

	case webapi.ActionLaunch:
		function := mustPopOptionalFunctionParam(w, r, s, query)
		instance := mustPopOptionalParam(w, r, s, query, webapi.ParamInstance)
		debug := mustPopOptionalParam(w, r, s, query, webapi.ParamDebug)
		mustNotHaveParams(w, r, s, query)

		switch mustParseContentType(w, r, s) {
		case "":
			mustNotHaveContent(w, r, s)
			handleLaunch(w, r, s, server.OpLaunchRef, nil, key, function, instance, debug)

		case webapi.ContentTypeWebAssembly:
			mustHaveContentLength(w, r, s)
			handleLaunchContent(w, r, s, key, function, instance, debug)

		default:
			respondUnsupportedMediaType(w, r, s)
		}

	case webapi.ActionUnref:
		mustNotHaveParams(w, r, s, query)
		mustNotHaveContentType(w, r, s)
		mustNotHaveContent(w, r, s)
		handleModuleUnref(w, r, s, key)

	default:
		respondUnsupportedAction(w, r, s)
	}
}

func handleGetModuleSource(w http.ResponseWriter, r *http.Request, s *webserver, source server.Source, key string) {
	query := mustParseQuery(w, r, s)

	switch mustPopParam(w, r, s, query, webapi.ParamAction) {
	case webapi.ActionCall:
		function := mustPopOptionalFunctionParam(w, r, s, query)
		debug := mustPopOptionalParam(w, r, s, query, webapi.ParamDebug)
		mustNotHaveParams(w, r, s, query)
		handleCallWebsocket(w, r, s, source, key, function, debug)

	default:
		respondUnsupportedAction(w, r, s)
	}
}

func handlePostModuleSource(w http.ResponseWriter, r *http.Request, s *webserver, source server.Source, key string) {
	query := mustParseQuery(w, r, s)

	switch mustPopParam(w, r, s, query, webapi.ParamAction) {
	case webapi.ActionCall:
		function := mustPopOptionalFunctionParam(w, r, s, query)
		debug := mustPopOptionalParam(w, r, s, query, webapi.ParamDebug)
		mustNotHaveParams(w, r, s, query)
		handleCallPost(w, r, s, server.OpCallSource, false, source, key, function, debug)

	case webapi.ActionLaunch:
		function := mustPopOptionalFunctionParam(w, r, s, query)
		instance := mustPopOptionalParam(w, r, s, query, webapi.ParamInstance)
		debug := mustPopOptionalParam(w, r, s, query, webapi.ParamDebug)
		mustNotHaveParams(w, r, s, query)
		mustNotHaveContentType(w, r, s)
		mustNotHaveContent(w, r, s)
		handleLaunch(w, r, s, server.OpLaunchSource, source, key, function, instance, debug)

	default:
		respondUnsupportedAction(w, r, s)
	}
}

func handleGetInstances(w http.ResponseWriter, r *http.Request, s *webserver) {
	mustNotHaveQuery(w, r, s)
	mustAcceptJSON(w, r, s)
	handleInstances(w, r, s)
}

func handleGetInstance(w http.ResponseWriter, r *http.Request, s *webserver, instance string) {
	query := mustParseQuery(w, r, s)

	switch mustPopParam(w, r, s, query, webapi.ParamAction) {
	case webapi.ActionIO:
		mustNotHaveParams(w, r, s, query)
		handleIOWebsocket(w, r, s, instance)

	default:
		respondUnsupportedAction(w, r, s)
	}
}

func handlePostInstance(w http.ResponseWriter, r *http.Request, s *webserver, instance string) {
	query := mustParseQuery(w, r, s)

	switch mustPopParam(w, r, s, query, webapi.ParamAction) {
	case webapi.ActionIO:
		mustNotHaveParams(w, r, s, query)
		handleIOPost(w, r, s, instance)

	case webapi.ActionStatus:
		mustNotHaveParams(w, r, s, query)
		handleInstance(w, r, s, server.OpInstanceStatus, (*server.Server).InstanceStatus, instance)

	case webapi.ActionWait:
		mustNotHaveParams(w, r, s, query)
		handleInstance(w, r, s, server.OpInstanceWait, (*server.Server).WaitInstance, instance)

	case webapi.ActionSuspend:
		mustNotHaveParams(w, r, s, query)
		handleInstance(w, r, s, server.OpInstanceSuspend, (*server.Server).SuspendInstance, instance)

	case webapi.ActionResume:
		debug := mustPopOptionalParam(w, r, s, query, webapi.ParamDebug)
		mustNotHaveParams(w, r, s, query)
		handleResume(w, r, s, instance, debug)

	case webapi.ActionSnapshot:
		mustNotHaveParams(w, r, s, query)
		handleSnapshot(w, r, s, instance)

	default:
		respondUnsupportedAction(w, r, s)
	}
}

// Action handlers.  Check authorization if needed, and serve the response.

func handleStatic(w http.ResponseWriter, r *http.Request, s *webserver, contentLength string, content []byte) {
	w.Header().Set("Cache-Control", cacheControlStatic)
	w.Header().Set(webapi.HeaderContentLength, contentLength)
	w.Header().Set(webapi.HeaderContentType, contentTypeJSON)
	w.Write(content)
}

func handleModules(w http.ResponseWriter, r *http.Request, s *webserver) {
	ctx := server.ContextWithOp(r.Context(), server.OpModuleList)
	wr := &requestResponseWriter{w, r}
	pri := mustParseAuthorizationHeader(ctx, wr, s)

	refs, err := s.Server.ModuleRefs(ctx, pri)
	if err != nil {
		respondServerError(ctx, wr, s, pri, "", "", "", "", err)
		return
	}

	sort.Sort(refs)

	if refs == nil {
		refs = []webapi.ModuleRef{} // For JSON.
	}

	w.Header().Set(webapi.HeaderContentType, contentTypeJSON)

	json.NewEncoder(w).Encode(&webapi.ModuleRefs{Modules: refs})
}

func handleModuleGet(w http.ResponseWriter, r *http.Request, s *webserver, key string) {
	ctx := server.ContextWithOp(r.Context(), server.OpModuleDownload)
	wr := &requestResponseWriter{w, r}
	pri := mustParseAuthorizationHeader(ctx, wr, s)

	content, length, err := s.Server.ModuleContent(ctx, pri, key)
	if err != nil {
		respondServerError(ctx, wr, s, pri, "", key, "", "", err)
		panic(nil)
	}
	defer content.Close()

	w.Header().Set(webapi.HeaderContentLength, strconv.FormatInt(length, 10))
	w.Header().Set(webapi.HeaderContentType, webapi.ContentTypeWebAssembly)
	w.WriteHeader(http.StatusOK)

	if r.Method != "HEAD" {
		io.Copy(w, content)
	}
}

func handleModulePut(w http.ResponseWriter, r *http.Request, s *webserver, key string) {
	ctx := server.ContextWithOp(r.Context(), server.OpModuleUpload)
	wr := &requestResponseWriter{w, r}
	pri := mustParseAuthorizationHeader(ctx, wr, s)

	err := s.Server.UploadModule(ctx, pri, key, mustDecodeContent(ctx, wr, s, pri), r.ContentLength)
	if err != nil {
		respondServerError(ctx, wr, s, pri, "", key, "", "", err)
		panic(nil)
	}

	w.WriteHeader(http.StatusCreated)
}

func handleModuleUnref(w http.ResponseWriter, r *http.Request, s *webserver, key string) {
	ctx := server.ContextWithOp(r.Context(), server.OpModuleUnref)
	wr := &requestResponseWriter{w, r}
	pri := mustParseAuthorizationHeader(ctx, wr, s)

	err := s.Server.UnrefModule(ctx, pri, key)
	if err != nil {
		respondServerError(ctx, wr, s, pri, "", key, "", "", err)
		panic(nil)
	}

	w.WriteHeader(http.StatusNoContent)
}

func handleCallPost(w http.ResponseWriter, r *http.Request, s *webserver, op detail.Op, content bool, source server.Source, key, function, debug string) {
	ctx := server.ContextWithOp(r.Context(), op) // TODO: detail: post
	wr := &requestResponseWriter{w, r}

	var (
		pri      *server.PrincipalKey
		progHash string
		inst     *server.Instance
		err      error
	)

	switch {
	case content:
		pri = mustParseOptionalAuthorizationHeader(ctx, wr, s)
		progHash = key

		inst, err = s.Server.UploadModuleInstance(ctx, pri, key, mustDecodeContent(ctx, wr, s, pri), r.ContentLength, function, "", debug)
		if err != nil {
			respondServerError(ctx, wr, s, pri, "", key, function, "", err)
			return
		}

	case source == nil:
		pri = mustParseAuthorizationHeader(ctx, wr, s)
		progHash = key

		inst, err = s.Server.CreateInstance(ctx, pri, key, function, "", debug)
		if err != nil {
			respondServerError(ctx, wr, s, pri, "", key, function, "", err)
			return
		}

	default:
		pri = mustParseOptionalAuthorizationHeader(ctx, wr, s)

		progHash, inst, err = s.Server.SourceModuleInstance(ctx, pri, source, key, function, "", debug)
		if err != nil {
			// TODO: find out module hash
			respondServerError(ctx, wr, s, pri, key, "", function, "", err)
			return
		}
	}

	if pri == nil {
		defer inst.Kill(s.Server)
	}

	w.Header().Set("Trailer", webapi.HeaderStatus)

	if debug != "" {
		w.Header().Set(webapi.HeaderDebug, inst.Status().Debug)
	}

	if pri != nil {
		if source != nil {
			w.Header().Set(webapi.HeaderLocation, s.pathModuleRefs+progHash)
		}
		w.Header().Set(webapi.HeaderInstance, inst.ID())
	}

	if pri != nil && source != nil {
		w.WriteHeader(http.StatusCreated)
	} else {
		w.WriteHeader(http.StatusOK)
	}

	disconnected := inst.Connect(ctx, r.Body, w)
	status, _ := inst.Run(ctx, s.Server)
	<-disconnected

	statusJSON := statusInternalServerErrorJSON
	if data, err := serverapi.MarshalJSON(&status); err == nil {
		statusJSON = string(data)
	} else {
		reportInternalError(ctx, s, pri, "", "", "", inst.ID(), err)
	}

	w.Header().Set(webapi.HeaderStatus, statusJSON)
}

func handleCallWebsocket(response http.ResponseWriter, request *http.Request, s *webserver, source server.Source, key, function, debug string) {
	ctx, cancel := context.WithCancel(request.Context())
	defer cancel()

	conn, err := websocketUpgrader.Upgrade(response, request, nil)
	if err != nil {
		reportProtocolError(ctx, s, nil, err)
		return
	}
	defer conn.Close()

	origCloseHandler := conn.CloseHandler()
	conn.SetCloseHandler(func(code int, text string) error {
		cancel()
		return origCloseHandler(code, text)
	})

	conn.SetReadLimit(maxWebsocketRequestSize)

	var r webapi.Call

	err = conn.ReadJSON(&r)
	if err != nil {
		if _, ok := err.(net.Error); ok {
			reportNetworkError(ctx, s, err)
			return
		}
		reportProtocolError(ctx, s, nil, err)
		return
	}

	conn.SetReadLimit(0)

	var content bool

	switch r.ContentType {
	case webapi.ContentTypeWebAssembly:
		if source != nil {
			conn.WriteMessage(websocket.CloseMessage, websocketUnsupportedContent)
			reportProtocolError(ctx, s, nil, errUnsupportedWebsocketContent)
			return
		}

		ctx = server.ContextWithOp(ctx, server.OpCallUpload) // TODO: detail: websocket
		content = true

	case "":
		if source == nil {
			ctx = server.ContextWithOp(ctx, server.OpCallRef) // TODO: detail: websocket
		} else {
			ctx = server.ContextWithOp(ctx, server.OpCallSource) // TODO: detail: websocket
		}

	default:
		conn.WriteMessage(websocket.CloseMessage, websocketUnsupportedContentType)
		reportProtocolError(ctx, s, nil, errUnsupportedWebsocketContentType)
		return
	}

	w := newWebsocketWriter(conn)

	var (
		pri      *server.PrincipalKey
		progHash string
		inst     *server.Instance
	)

	switch {
	case content:
		pri = mustParseOptionalAuthorization(ctx, w, s, r.Authorization)
		progHash = key

		frameType, frame, err := conn.NextReader()
		if err != nil {
			reportNetworkError(ctx, s, err)
			return
		}
		if frameType != websocket.BinaryMessage {
			conn.WriteMessage(websocket.CloseMessage, websocketUnsupportedData)
			reportProtocolError(ctx, s, nil, errWrongWebsocketMessageType)
			return
		}

		inst, err = s.Server.UploadModuleInstance(ctx, pri, key, ioutil.NopCloser(frame), r.ContentLength, function, "", debug)
		if err != nil {
			respondServerError(ctx, w, s, pri, "", key, function, "", err)
			return
		}

	case source == nil:
		pri = mustParseAuthorization(ctx, w, s, r.Authorization)
		progHash = key

		inst, err = s.Server.CreateInstance(ctx, pri, key, function, "", debug)
		if err != nil {
			respondServerError(ctx, w, s, pri, "", key, function, "", err)
			return
		}

	default:
		pri = mustParseOptionalAuthorization(ctx, w, s, r.Authorization)

		progHash, inst, err = s.Server.SourceModuleInstance(ctx, pri, source, key, function, "", debug)
		if err != nil {
			// TODO: find out module hash
			respondServerError(ctx, w, s, pri, key, "", function, "", err)
			return
		}
	}

	if pri == nil {
		defer inst.Kill(s.Server)
	}

	var reply webapi.CallConnection

	if pri != nil {
		if source != nil {
			reply.Location = s.pathModuleRefs + progHash
		}
		reply.Instance = inst.ID()
	}

	if debug != "" {
		reply.Debug = inst.Status().Debug
	}

	err = conn.WriteJSON(&reply)
	if err != nil {
		reportNetworkError(ctx, s, err)
		return
	}

	disconnected := inst.Connect(ctx, newWebsocketReadCanceler(conn, cancel), w)
	status, _ := inst.Run(ctx, s.Server)
	<-disconnected

	// TODO: send ConnectionStatus
	statusJSON, err := serverapi.MarshalJSON(&status)
	if err != nil {
		reportInternalError(ctx, s, pri, "", "", "", inst.ID(), err)
		statusJSON = []byte(statusInternalServerErrorJSON)
	}

	if conn.WriteMessage(websocket.TextMessage, statusJSON) == nil {
		conn.WriteMessage(websocket.CloseMessage, websocketNormalClosure)
	}
}

func handleLaunch(w http.ResponseWriter, r *http.Request, s *webserver, op detail.Op, source server.Source, key, function, instID, debug string) {
	ctx := server.ContextWithOp(r.Context(), op)
	wr := &requestResponseWriter{w, r}
	pri := mustParseAuthorizationHeader(ctx, wr, s)

	var (
		progHash string
		inst     *server.Instance
		err      error
	)

	if source == nil {
		progHash = key

		inst, err = s.Server.CreateInstance(ctx, pri, key, function, instID, debug)
		if err != nil {
			respondServerError(ctx, wr, s, pri, "", key, function, "", err)
			return
		}
	} else {
		progHash, inst, err = s.Server.SourceModuleInstance(ctx, pri, source, key, function, instID, debug)
		if err != nil {
			respondServerError(ctx, wr, s, pri, key, "", function, "", err)
			return
		}
	}

	go s.runBackgroundInstance(inst)

	if debug != "" {
		w.Header().Set(webapi.HeaderDebug, inst.Status().Debug)
	}

	w.Header().Set(webapi.HeaderInstance, inst.ID())

	if source == nil {
		w.WriteHeader(http.StatusOK)
	} else {
		w.Header().Set(webapi.HeaderLocation, s.pathModuleRefs+progHash)
		w.WriteHeader(http.StatusCreated)
	}
}

func handleLaunchContent(w http.ResponseWriter, r *http.Request, s *webserver, key, function, instID, debug string) {
	ctx := server.ContextWithOp(r.Context(), server.OpLaunchUpload)
	wr := &requestResponseWriter{w, r}
	pri := mustParseAuthorizationHeader(ctx, wr, s)

	inst, err := s.Server.UploadModuleInstance(ctx, pri, key, mustDecodeContent(ctx, wr, s, pri), r.ContentLength, function, instID, debug)
	if err != nil {
		respondServerError(ctx, wr, s, pri, "", key, function, "", err)
		return
	}

	go s.runBackgroundInstance(inst)

	if debug != "" {
		w.Header().Set(webapi.HeaderDebug, inst.Status().Debug)
	}

	w.Header().Set(webapi.HeaderInstance, inst.ID())
	w.Header().Set(webapi.HeaderLocation, s.pathModuleRefs+key)
	w.WriteHeader(http.StatusCreated)
}

func handleInstances(w http.ResponseWriter, r *http.Request, s *webserver) {
	ctx := server.ContextWithOp(r.Context(), server.OpInstanceList)
	wr := &requestResponseWriter{w, r}
	pri := mustParseAuthorizationHeader(ctx, wr, s)

	instances, err := s.Server.Instances(ctx, pri)
	if err != nil {
		respondServerError(ctx, wr, s, pri, "", "", "", "", err)
		return
	}

	sort.Sort(instances)

	if instances == nil {
		instances = []server.InstanceStatus{}
	}

	w.Header().Set(webapi.HeaderContentType, contentTypeJSON)

	serverapi.JSONMarshaler.Marshal(w, &serverapi.Instances{
		Instances: instances,
	})
}

func handleInstance(w http.ResponseWriter, r *http.Request, s *webserver, op detail.Op, method instanceMethod, instID string) {
	ctx := server.ContextWithOp(r.Context(), op)
	wr := &requestResponseWriter{w, r}
	pri := mustParseAuthorizationHeader(ctx, wr, s)

	status, err := method(s.Server, ctx, pri, instID)
	if err != nil {
		respondServerError(ctx, wr, s, pri, "", "", "", instID, err)
		return
	}

	statusJSON := statusInternalServerErrorJSON
	if data, err := serverapi.MarshalJSON(&status); err == nil {
		statusJSON = string(data)
	} else {
		reportInternalError(ctx, s, pri, "", "", "", instID, err)
	}

	w.Header().Set(webapi.HeaderStatus, statusJSON)
	w.WriteHeader(http.StatusNoContent)
}

func handleResume(w http.ResponseWriter, r *http.Request, s *webserver, instID, debug string) {
	ctx := server.ContextWithOp(r.Context(), server.OpInstanceResume)
	wr := &requestResponseWriter{w, r}
	pri := mustParseAuthorizationHeader(ctx, wr, s)

	inst, err := s.Server.ResumeInstance(ctx, pri, instID, debug)
	if err != nil {
		respondServerError(ctx, wr, s, pri, "", "", "", instID, err)
		return
	}

	go s.runBackgroundInstance(inst)

	if debug != "" {
		w.Header().Set(webapi.HeaderDebug, inst.Status().Debug)
	}

	w.WriteHeader(http.StatusNoContent)
}

func handleIOPost(w http.ResponseWriter, r *http.Request, s *webserver, instID string) {
	ctx := server.ContextWithOp(r.Context(), server.OpInstanceConnect) // TODO: detail: post
	wr := &requestResponseWriter{w, r}
	pri := mustParseAuthorizationHeader(ctx, wr, s)

	content := mustDecodeContent(ctx, wr, s, pri)
	defer content.Close()

	connIO, err := s.Server.InstanceConnection(ctx, pri, instID)
	if err != nil {
		respondServerError(ctx, wr, s, pri, "", "", "", instID, err)
		return
	}
	if connIO == nil {
		w.WriteHeader(http.StatusConflict)
		return
	}

	w.Header().Set("Trailer", webapi.HeaderStatus)
	w.WriteHeader(http.StatusOK)

	status, err := connIO(ctx, content, w)
	if err != nil {
		// Network error has already been reported by connIO.
		return
	}

	statusJSON := statusInternalServerErrorJSON
	if data, err := serverapi.MarshalJSON(&status); err == nil {
		statusJSON = string(data)
	} else {
		reportInternalError(ctx, s, pri, "", "", "", instID, err)
	}

	w.Header().Set(webapi.HeaderStatus, statusJSON)
}

func handleIOWebsocket(response http.ResponseWriter, request *http.Request, s *webserver, instID string) {
	ctx := server.ContextWithOp(request.Context(), server.OpInstanceConnect) // TODO: detail: websocket

	conn, err := websocketUpgrader.Upgrade(response, request, nil)
	if err != nil {
		reportProtocolError(ctx, s, nil, err)
		return
	}
	defer conn.Close()

	conn.SetReadLimit(maxWebsocketRequestSize)

	var r webapi.IO

	err = conn.ReadJSON(&r)
	if err != nil {
		if _, ok := err.(net.Error); ok {
			reportNetworkError(ctx, s, err)
			return
		}
		reportProtocolError(ctx, s, nil, err)
		return
	}

	conn.SetReadLimit(0)

	w := newWebsocketWriter(conn)
	pri := mustParseAuthorization(ctx, w, s, r.Authorization)

	connIO, err := s.Server.InstanceConnection(ctx, pri, instID)
	if err != nil {
		respondServerError(ctx, w, s, pri, "", "", "", instID, err)
		conn.WriteMessage(websocket.CloseMessage, websocketNormalClosure)
		return
	}

	reply := &serverapi.IOConnection{
		Connected: connIO != nil,
	}

	err = conn.WriteMessage(websocket.TextMessage, serverapi.MustMarshalJSON(reply))
	if err != nil {
		reportNetworkError(ctx, s, err)
		return
	}

	if connIO == nil {
		conn.WriteMessage(websocket.CloseMessage, websocketNotConnected)
		return
	}

	goodbye := &serverapi.ConnectionStatus{}

	goodbye.Status, err = connIO(ctx, newWebsocketReader(conn), newWebsocketWriter(conn))
	if err != nil {
		// Network error has already been reported by connIO.
		return
	}

	data, err := serverapi.MarshalJSON(goodbye)
	if err != nil {
		conn.WriteMessage(websocket.CloseMessage, websocketInternalServerErr)
		reportInternalError(ctx, s, pri, "", "", "", "", err)
		return
	}

	err = conn.WriteMessage(websocket.TextMessage, data)
	if err != nil {
		reportNetworkError(ctx, s, err)
		return
	}

	conn.WriteMessage(websocket.CloseMessage, websocketNormalClosure)
}

func handleSnapshot(w http.ResponseWriter, r *http.Request, s *webserver, instID string) {
	ctx := server.ContextWithOp(r.Context(), server.OpInstanceSnapshot)
	wr := &requestResponseWriter{w, r}
	pri := mustParseAuthorizationHeader(ctx, wr, s)

	moduleKey, err := s.Server.InstanceModule(ctx, pri, instID)
	if err != nil {
		respondServerError(ctx, wr, s, pri, "", "", "", instID, err)
		return
	}

	w.Header().Set(webapi.HeaderLocation, s.pathModuleRefs+moduleKey)
	w.WriteHeader(http.StatusCreated)
}
