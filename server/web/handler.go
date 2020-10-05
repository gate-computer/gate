// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package web

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

	"gate.computer/gate/internal/jsonproto"
	"gate.computer/gate/internal/principal"
	api "gate.computer/gate/internal/webserverapi"
	"gate.computer/gate/server"
	serverapi "gate.computer/gate/server/api"
	"gate.computer/gate/server/detail"
	"gate.computer/gate/server/event"
	"gate.computer/gate/webapi"
	"github.com/gorilla/websocket"
)

const maxWebsocketRequestSize = 4096

type errorWriter interface {
	SetHeader(key, value string)
	WriteError(status int, text string)
}

type instanceMethod func(s *server.Server, ctx context.Context, instance string) error
type instanceStatusMethod func(s *server.Server, ctx context.Context, instance string) (serverapi.Status, error)
type instanceWaiterMethod func(s *server.Server, ctx context.Context, instance string) (*server.Instance, error)

type webserver struct {
	Config

	identity       string // JWT audience.
	pathModuleRefs string
}

func NewHandler(pattern string, config Config) http.Handler {
	s := &webserver{
		Config: config,
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
	patternAPI := p + webapi.Path                                       // host/path/api/
	patternModule := p + webapi.PathModule                              // host/path/api/module
	patternModules := p + webapi.PathModules                            // host/path/api/module/
	patternModuleRef := p + webapi.PathModules + webapi.ModuleRefSource // host/path/api/module/hash
	patternModuleRefs := p + webapi.PathModuleRefs                      // host/path/api/module/hash/
	patternInstances := p + webapi.PathInstances                        // host/path/api/instance/
	patternInstance := patternInstances[:len(patternInstances)-1]       // host/path/api/instance

	p = strings.TrimLeftFunc(p, func(r rune) bool { return r != '/' }) // /path
	pathAPI := p + webapi.Path                                         // /path/api/
	pathModule := p + webapi.PathModule                                // /path/api/module
	pathModules := p + webapi.PathModules                              // /path/api/module/
	pathModuleRef := p + webapi.PathModules + webapi.ModuleRefSource   // /path/api/module/hash
	s.pathModuleRefs = p + webapi.PathModuleRefs                       // /path/api/module/hash/
	pathInstances := p + webapi.PathInstances                          // /path/api/instance/
	pathInstance := pathInstances[:len(pathInstances)-1]               // /path/api/instance

	s.identity = "https://" + s.Authority + p + webapi.Path // https://authority/path/api/

	mux := http.NewServeMux()
	mux.HandleFunc(patternAPI, newOpaqueHandler(s, pathAPI))
	mux.HandleFunc(patternModule, newOpaqueHandler(s, pathModule))
	mux.HandleFunc(patternInstance, newOpaqueHandler(s, pathInstance))
	mux.HandleFunc(patternInstances, newInstanceHandler(s, pathInstances))
	mux.HandleFunc(patternModuleRef, newOpaqueHandler(s, pathModuleRef))
	mux.HandleFunc(patternModuleRefs, newModuleRefHandler(s))

	moduleSources := []string{webapi.ModuleRefSource}

	for relURI, source := range s.ModuleSources {
		patternSource := patternModule + relURI // host/path/api/module/source
		patternSourceDir := patternSource + "/" // host/path/api/module/source/

		pathSource := pathModule + relURI // /path/api/module/source
		pathSourceDir := pathSource + "/" // /path/api/module/source/

		mux.HandleFunc(patternSource, newOpaqueHandler(s, pathSource))
		mux.HandleFunc(patternSourceDir, newModuleSourceHandler(s, pathModule, pathSourceDir, source))

		moduleSources = append(moduleSources, strings.TrimLeft(relURI, "/"))
	}

	sort.Strings(moduleSources)
	mux.HandleFunc(patternModules, newStaticHandler(s, pathModules, moduleSources))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := server.ContextWithRequestAddr(r.Context(), s.NewRequestID(r), r.RemoteAddr)

		s.Server.Monitor(&event.IfaceAccess{
			Ctx: server.ContextDetail(ctx),
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
		setAccessControl(w, r, "GET, HEAD, "+methods)

		switch r.Method {
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

func newModuleRefHandler(s *webserver) http.HandlerFunc {
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

func newModuleSourceHandler(s *webserver, sourceURIBase, sourcePath string, source server.Source) http.HandlerFunc {
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
			setAccessControl(w, r, "GET, HEAD, "+methods)

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
		headersGet  = join(webapi.HeaderAuthorization)
		headersPost = join(webapi.HeaderAuthorization, webapi.HeaderContentType)
		exposed     = join(webapi.HeaderStatus)
	)

	return func(w http.ResponseWriter, r *http.Request) {
		if len(r.URL.Path) == len(instancesPath) {
			// Instance directory listing

			methods := "GET, HEAD, OPTIONS"
			setAccessControlAllowHeaders(w, r, methods, headersGet)

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
			setAccessControlAllowExposeHeaders(w, r, methods, headersPost, exposed)

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
	handleModuleList(w, r, s)
}

func handleGetModuleRef(w http.ResponseWriter, r *http.Request, s *webserver, key string) {
	query := mustParseOptionalQuery(w, r, s)
	ref := popOptionalActionParam(w, r, s, query, webapi.ActionRef)

	if _, found := query[webapi.ParamAction]; found {
		switch popLastParam(w, r, s, query, webapi.ParamAction) {
		case webapi.ActionCall:
			function := mustPopOptionalLastFunctionParam(w, r, s, query)
			debug := popOptionalLastDebugParam(w, r, s, query)
			mustNotHaveParams(w, r, s, query)
			handleCallWebsocket(w, r, s, ref, nil, key, function, debug)

		default:
			respondUnsupportedAction(w, r, s)
		}
	} else {
		if ref {
			respondUnsupportedAction(w, r, s)
		} else {
			mustAcceptWebAssembly(w, r, s)
			mustNotHaveParams(w, r, s, query)
			handleModuleDownload(w, r, s, key)
		}
	}
}

func handlePutModuleRef(w http.ResponseWriter, r *http.Request, s *webserver, key string) {
	mustHaveContentType(w, r, s, webapi.ContentTypeWebAssembly)
	mustHaveContentLength(w, r, s)
	query := mustParseOptionalQuery(w, r, s)
	ref := popOptionalActionParam(w, r, s, query, webapi.ActionRef)
	suspend := popOptionalActionParam(w, r, s, query, webapi.ActionSuspend)

	if _, found := query[webapi.ParamAction]; found {
		switch popLastParam(w, r, s, query, webapi.ParamAction) {
		case webapi.ActionCall:
			function := mustPopOptionalLastFunctionParam(w, r, s, query)
			debug := popOptionalLastDebugParam(w, r, s, query)
			mustNotHaveParams(w, r, s, query)
			handleCall(w, r, s, server.OpCallUpload, ref, true, nil, key, function, debug)

		case webapi.ActionLaunch:
			function := mustPopOptionalLastFunctionParam(w, r, s, query)
			instance := popOptionalLastParam(w, r, s, query, webapi.ParamInstance)
			debug := popOptionalLastDebugParam(w, r, s, query)
			mustNotHaveParams(w, r, s, query)
			handleLaunchUpload(w, r, s, ref, key, function, instance, debug, suspend)

		default:
			respondUnsupportedAction(w, r, s)
		}
	} else {
		mustNotHaveParams(w, r, s, query)
		handleModuleUpload(w, r, s, ref, key)
	}
}

func handlePostModuleRef(w http.ResponseWriter, r *http.Request, s *webserver, key string) {
	query := mustParseQuery(w, r, s)
	suspend := popOptionalActionParam(w, r, s, query, webapi.ActionSuspend)

	switch popLastParam(w, r, s, query, webapi.ParamAction) {
	case webapi.ActionCall:
		function := mustPopOptionalLastFunctionParam(w, r, s, query)
		debug := popOptionalLastDebugParam(w, r, s, query)
		mustNotHaveParams(w, r, s, query)
		handleCall(w, r, s, server.OpCallExtant, false, false, nil, key, function, debug)

	case webapi.ActionLaunch:
		function := mustPopOptionalLastFunctionParam(w, r, s, query)
		instance := popOptionalLastParam(w, r, s, query, webapi.ParamInstance)
		debug := popOptionalLastDebugParam(w, r, s, query)
		mustNotHaveParams(w, r, s, query)
		mustNotHaveContent(w, r, s)
		handleLaunch(w, r, s, server.OpLaunchExtant, false, nil, key, function, instance, debug, suspend)

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
	ref := popOptionalActionParam(w, r, s, query, webapi.ActionRef)

	switch popLastParam(w, r, s, query, webapi.ParamAction) {
	case webapi.ActionCall:
		function := mustPopOptionalLastFunctionParam(w, r, s, query)
		debug := popOptionalLastDebugParam(w, r, s, query)
		mustNotHaveParams(w, r, s, query)
		handleCallWebsocket(w, r, s, ref, source, key, function, debug)

	default:
		respondUnsupportedAction(w, r, s)
	}
}

func handlePostModuleSource(w http.ResponseWriter, r *http.Request, s *webserver, source server.Source, key string) {
	query := mustParseQuery(w, r, s)
	ref := popOptionalActionParam(w, r, s, query, webapi.ActionRef)
	suspend := popOptionalActionParam(w, r, s, query, webapi.ActionSuspend)

	if _, found := query[webapi.ParamAction]; found {
		switch popLastParam(w, r, s, query, webapi.ParamAction) {
		case webapi.ActionCall:
			function := mustPopOptionalLastFunctionParam(w, r, s, query)
			debug := popOptionalLastDebugParam(w, r, s, query)
			mustNotHaveParams(w, r, s, query)
			handleCall(w, r, s, server.OpCallSource, ref, false, source, key, function, debug)

		case webapi.ActionLaunch:
			function := mustPopOptionalLastFunctionParam(w, r, s, query)
			instance := popOptionalLastParam(w, r, s, query, webapi.ParamInstance)
			debug := popOptionalLastDebugParam(w, r, s, query)
			mustNotHaveParams(w, r, s, query)
			mustNotHaveContentType(w, r, s)
			mustNotHaveContent(w, r, s)
			handleLaunch(w, r, s, server.OpLaunchSource, ref, source, key, function, instance, debug, suspend)

		default:
			respondUnsupportedAction(w, r, s)
		}
	} else {
		mustNotHaveParams(w, r, s, query)
		mustNotHaveContentType(w, r, s)
		mustNotHaveContent(w, r, s)
		handleModuleSource(w, r, s, ref, source, key)
	}
}

func handleGetInstances(w http.ResponseWriter, r *http.Request, s *webserver) {
	mustNotHaveQuery(w, r, s)
	mustAcceptJSON(w, r, s)
	handleInstanceList(w, r, s)
}

func handleGetInstance(w http.ResponseWriter, r *http.Request, s *webserver, instance string) {
	query := mustParseQuery(w, r, s)

	switch popLastParam(w, r, s, query, webapi.ParamAction) {
	case webapi.ActionIO:
		mustNotHaveParams(w, r, s, query)
		handleInstanceConnectWebsocket(w, r, s, instance)

	default:
		respondUnsupportedAction(w, r, s)
	}
}

func handlePostInstance(w http.ResponseWriter, r *http.Request, s *webserver, instance string) {
	query := mustParseQuery(w, r, s)
	wait := popOptionalActionParam(w, r, s, query, webapi.ActionWait)

	if wait && len(query[webapi.ParamAction]) == 0 {
		handleInstanceStatus(w, r, s, server.OpInstanceWait, (*server.Server).WaitInstance, instance)
		return
	}

	switch popLastParam(w, r, s, query, webapi.ParamAction) {
	case webapi.ActionIO:
		mustNotHaveParams(w, r, s, query)
		handleInstanceConnect(w, r, s, instance)

	case webapi.ActionStatus:
		mustNotHaveParams(w, r, s, query)
		handleInstanceStatus(w, r, s, server.OpInstanceStatus, (*server.Server).InstanceStatus, instance)

	case webapi.ActionKill:
		mustNotHaveParams(w, r, s, query)
		handleInstanceWaiter(w, r, s, server.OpInstanceKill, (*server.Server).KillInstance, instance, wait)

	case webapi.ActionSuspend:
		mustNotHaveParams(w, r, s, query)
		handleInstanceWaiter(w, r, s, server.OpInstanceSuspend, (*server.Server).SuspendInstance, instance, wait)

	case webapi.ActionResume:
		function := mustPopOptionalLastFunctionParam(w, r, s, query)
		debug := popOptionalLastDebugParam(w, r, s, query)
		mustNotHaveParams(w, r, s, query)
		handleInstanceResume(w, r, s, function, instance, debug)

	case webapi.ActionSnapshot:
		mustNotHaveParams(w, r, s, query)
		handleInstanceSnapshot(w, r, s, instance)

	case webapi.ActionDelete:
		mustNotHaveParams(w, r, s, query)
		handleInstance(w, r, s, server.OpInstanceDelete, (*server.Server).DeleteInstance, instance)

	case webapi.ActionDebug:
		mustNotHaveParams(w, r, s, query)
		handleInstanceDebug(w, r, s, instance)

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

func handleModuleList(w http.ResponseWriter, r *http.Request, s *webserver) {
	ctx := server.ContextWithOp(r.Context(), server.OpModuleList)
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	refs, err := s.Server.ModuleRefs(ctx)
	if err != nil {
		respondServerError(ctx, wr, s, "", "", "", "", err)
		return
	}

	sort.Sort(refs)

	if refs.Modules == nil {
		refs.Modules = []webapi.ModuleRef{} // For JSON.
	}

	w.Header().Set(webapi.HeaderContentType, contentTypeJSON)

	json.NewEncoder(w).Encode(refs)
}

func handleModuleDownload(w http.ResponseWriter, r *http.Request, s *webserver, key string) {
	ctx := server.ContextWithOp(r.Context(), server.OpModuleDownload)
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	content, length, err := s.Server.ModuleContent(ctx, key)
	if err != nil {
		respondServerError(ctx, wr, s, "", key, "", "", err)
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

func handleModuleUpload(w http.ResponseWriter, r *http.Request, s *webserver, ref bool, key string) {
	ctx := server.ContextWithOp(r.Context(), server.OpModuleUpload)
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, ref)

	_, err := s.Server.UploadModule(ctx, ref, key, mustDecodeContent(ctx, wr, s), r.ContentLength)
	if err != nil {
		respondServerError(ctx, wr, s, "", key, "", "", err)
		panic(nil)
	}

	if ref {
		w.WriteHeader(http.StatusCreated)
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleModuleUnref(w http.ResponseWriter, r *http.Request, s *webserver, key string) {
	ctx := server.ContextWithOp(r.Context(), server.OpModuleUnref)
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	err := s.Server.UnrefModule(ctx, key)
	if err != nil {
		respondServerError(ctx, wr, s, "", key, "", "", err)
		panic(nil)
	}

	w.WriteHeader(http.StatusNoContent)
}

func handleModuleSource(w http.ResponseWriter, r *http.Request, s *webserver, ref bool, source server.Source, key string) {
	ctx := server.ContextWithOp(r.Context(), server.OpModuleSource)
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, ref)

	progHash, err := s.Server.SourceModule(ctx, ref, source, key)
	if err != nil {
		respondServerError(ctx, wr, s, key, "", "", "", err)
		return
	}

	if ref {
		w.Header().Set(webapi.HeaderLocation, s.pathModuleRefs+progHash)
		w.WriteHeader(http.StatusCreated)
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleCall(w http.ResponseWriter, r *http.Request, s *webserver, op detail.Op, ref, content bool, source server.Source, key, function string, debug bool) {
	ctx := server.ContextWithOp(r.Context(), op) // TODO: detail: post
	wr := &requestResponseWriter{w, r}

	var (
		progHash string
		inst     *server.Instance
		err      error
	)

	switch {
	case content:
		ctx = mustParseAuthorizationHeader(ctx, wr, s, ref)

		progHash = key
		inst, err = s.Server.UploadModuleInstance(ctx, mustDecodeContent(ctx, wr, s), r.ContentLength, key, ref, "", function, true, false, nil)
		if err != nil {
			respondServerError(ctx, wr, s, "", key, function, "", err)
			return
		}

	case source == nil:
		ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

		progHash = key
		inst, err = s.Server.CreateInstance(ctx, key, "", function, true, false, nil)
		if err != nil {
			respondServerError(ctx, wr, s, "", key, function, "", err)
			return
		}

	default:
		ctx = mustParseAuthorizationHeader(ctx, wr, s, ref)

		progHash, inst, err = s.Server.SourceModuleInstance(ctx, source, key, ref, "", function, true, false, nil)
		if err != nil {
			// TODO: find out module hash
			respondServerError(ctx, wr, s, key, "", function, "", err)
			return
		}
	}
	defer inst.Kill()

	w.Header().Set("Trailer", webapi.HeaderStatus)

	if principal.ContextID(ctx) != nil {
		if ref {
			w.Header().Set(webapi.HeaderLocation, s.pathModuleRefs+progHash)
		}
		w.Header().Set(webapi.HeaderInstance, inst.ID)
	}

	if ref {
		w.WriteHeader(http.StatusCreated)
	} else {
		w.WriteHeader(http.StatusOK)
	}

	inst.Connect(ctx, r.Body, w)
	status := inst.Wait(ctx)
	w.Header().Set(webapi.HeaderStatus, string(jsonproto.MustMarshal(&status)))
}

func handleCallWebsocket(response http.ResponseWriter, request *http.Request, s *webserver, ref bool, source server.Source, key, function string, debug bool) {
	ctx, cancel := context.WithCancel(request.Context())
	defer cancel()

	conn, err := websocketUpgrader.Upgrade(response, request, nil)
	if err != nil {
		reportProtocolError(ctx, s, err)
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
		reportProtocolError(ctx, s, err)
		return
	}

	conn.SetReadLimit(0)

	var content bool

	switch r.ContentType {
	case webapi.ContentTypeWebAssembly:
		if source != nil {
			conn.WriteMessage(websocket.CloseMessage, websocketUnsupportedContent)
			reportProtocolError(ctx, s, errUnsupportedWebsocketContent)
			return
		}

		ctx = server.ContextWithOp(ctx, server.OpCallUpload) // TODO: detail: websocket
		content = true

	case "":
		if source == nil {
			ctx = server.ContextWithOp(ctx, server.OpCallExtant) // TODO: detail: websocket
		} else {
			ctx = server.ContextWithOp(ctx, server.OpCallSource) // TODO: detail: websocket
		}

	default:
		conn.WriteMessage(websocket.CloseMessage, websocketUnsupportedContentType)
		reportProtocolError(ctx, s, errUnsupportedWebsocketContentType)
		return
	}

	w := newWebsocketWriter(conn)

	var (
		progHash string
		inst     *server.Instance
	)

	switch {
	case content:
		ctx = mustParseAuthorization(ctx, w, s, r.Authorization, ref)
		progHash = key

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

		inst, err = s.Server.UploadModuleInstance(ctx, ioutil.NopCloser(frame), r.ContentLength, key, ref, "", function, true, false, nil)
		if err != nil {
			respondServerError(ctx, w, s, "", key, function, "", err)
			return
		}

	case source == nil:
		ctx = mustParseAuthorization(ctx, w, s, r.Authorization, false)
		progHash = key

		inst, err = s.Server.CreateInstance(ctx, key, "", function, true, false, nil)
		if err != nil {
			respondServerError(ctx, w, s, "", key, function, "", err)
			return
		}

	default:
		ctx = mustParseAuthorization(ctx, w, s, r.Authorization, ref)

		progHash, inst, err = s.Server.SourceModuleInstance(ctx, source, key, ref, "", function, true, false, nil)
		if err != nil {
			// TODO: find out module hash
			respondServerError(ctx, w, s, key, "", function, "", err)
			return
		}
	}
	defer inst.Kill()

	var reply webapi.CallConnection

	if principal.ContextID(ctx) != nil {
		if ref {
			reply.Location = s.pathModuleRefs + progHash
		}
		reply.Instance = inst.ID
	}

	err = conn.WriteJSON(&reply)
	if err != nil {
		reportNetworkError(ctx, s, err)
		return
	}

	inst.Connect(ctx, newWebsocketReadCanceler(conn, cancel), w)
	status := inst.Wait(ctx)
	statusJSON := jsonproto.MustMarshal(&status) // TODO: send ConnectionStatus
	if conn.WriteMessage(websocket.TextMessage, statusJSON) == nil {
		conn.WriteMessage(websocket.CloseMessage, websocketNormalClosure)
	}
}

func handleLaunch(w http.ResponseWriter, r *http.Request, s *webserver, op detail.Op, ref bool, source server.Source, key, function, instID string, debug bool, suspend bool) {
	ctx := server.ContextWithOp(r.Context(), op)
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	var (
		progHash string
		inst     *server.Instance
		err      error
	)

	if source == nil {
		progHash = key

		inst, err = s.Server.CreateInstance(ctx, key, instID, function, false, suspend, nil)
		if err != nil {
			respondServerError(ctx, wr, s, "", key, function, "", err)
			return
		}
	} else {
		progHash, inst, err = s.Server.SourceModuleInstance(ctx, source, key, ref, instID, function, false, suspend, nil)
		if err != nil {
			respondServerError(ctx, wr, s, key, "", function, "", err)
			return
		}
	}

	w.Header().Set(webapi.HeaderInstance, inst.ID)

	if ref {
		w.Header().Set(webapi.HeaderLocation, s.pathModuleRefs+progHash)
		w.WriteHeader(http.StatusCreated)
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleLaunchUpload(w http.ResponseWriter, r *http.Request, s *webserver, ref bool, key, function, instID string, debug bool, suspend bool) {
	ctx := server.ContextWithOp(r.Context(), server.OpLaunchUpload)
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	inst, err := s.Server.UploadModuleInstance(ctx, mustDecodeContent(ctx, wr, s), r.ContentLength, key, ref, instID, function, false, suspend, nil)
	if err != nil {
		respondServerError(ctx, wr, s, "", key, function, "", err)
		return
	}

	w.Header().Set(webapi.HeaderInstance, inst.ID)

	if ref {
		w.Header().Set(webapi.HeaderLocation, s.pathModuleRefs+key)
		w.WriteHeader(http.StatusCreated)
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleInstanceList(w http.ResponseWriter, r *http.Request, s *webserver) {
	ctx := server.ContextWithOp(r.Context(), server.OpInstanceList)
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	instances, err := s.Server.Instances(ctx)
	if err != nil {
		respondServerError(ctx, wr, s, "", "", "", "", err)
		return
	}

	sort.Sort(instances)

	if instances.Instances == nil {
		instances.Instances = []serverapi.InstanceStatus{} // For JSON.
	}

	w.Header().Set(webapi.HeaderContentType, contentTypeJSON)

	jsonproto.Marshaler.Marshal(w, &instances)
}

func handleInstance(w http.ResponseWriter, r *http.Request, s *webserver, op detail.Op, method instanceMethod, instID string) {
	ctx := server.ContextWithOp(r.Context(), op)
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	err := method(s.Server, ctx, instID)
	if err != nil {
		respondServerError(ctx, wr, s, "", "", "", instID, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func handleInstanceStatus(w http.ResponseWriter, r *http.Request, s *webserver, op detail.Op, method instanceStatusMethod, instID string) {
	ctx := server.ContextWithOp(r.Context(), op)
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	status, err := method(s.Server, ctx, instID)
	if err != nil {
		respondServerError(ctx, wr, s, "", "", "", instID, err)
		return
	}

	w.Header().Set(webapi.HeaderStatus, string(jsonproto.MustMarshal(&status)))
	w.WriteHeader(http.StatusNoContent)
}

func handleInstanceWaiter(w http.ResponseWriter, r *http.Request, s *webserver, op detail.Op, method instanceWaiterMethod, instID string, wait bool) {
	ctx := server.ContextWithOp(r.Context(), op)
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	inst, err := method(s.Server, ctx, instID)
	if err != nil {
		respondServerError(ctx, wr, s, "", "", "", instID, err)
		return
	}

	if wait {
		status := inst.Wait(ctx)
		w.Header().Set(webapi.HeaderStatus, string(jsonproto.MustMarshal(&status)))
	}

	w.WriteHeader(http.StatusNoContent)
}

func handleInstanceResume(w http.ResponseWriter, r *http.Request, s *webserver, function, instID string, debug bool) {
	ctx := server.ContextWithOp(r.Context(), server.OpInstanceResume)
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	_, err := s.Server.ResumeInstance(ctx, instID, function, nil)
	if err != nil {
		respondServerError(ctx, wr, s, "", "", function, instID, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func handleInstanceConnect(w http.ResponseWriter, r *http.Request, s *webserver, instID string) {
	ctx := server.ContextWithOp(r.Context(), server.OpInstanceConnect) // TODO: detail: post
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	content := mustDecodeContent(ctx, wr, s)
	defer content.Close()

	inst, connIO, err := s.Server.InstanceConnection(ctx, instID)
	if err != nil {
		respondServerError(ctx, wr, s, "", "", "", instID, err)
		return
	}
	if connIO == nil {
		w.WriteHeader(http.StatusConflict)
		return
	}

	w.Header().Set("Trailer", webapi.HeaderStatus)
	w.WriteHeader(http.StatusOK)

	err = connIO(ctx, content, w)
	if err != nil {
		// Network error has already been reported by connIO.
		return
	}

	status := inst.Status()
	w.Header().Set(webapi.HeaderStatus, string(jsonproto.MustMarshal(&status)))
}

func handleInstanceConnectWebsocket(response http.ResponseWriter, request *http.Request, s *webserver, instID string) {
	ctx := server.ContextWithOp(request.Context(), server.OpInstanceConnect) // TODO: detail: websocket

	conn, err := websocketUpgrader.Upgrade(response, request, nil)
	if err != nil {
		reportProtocolError(ctx, s, err)
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
		reportProtocolError(ctx, s, err)
		return
	}

	conn.SetReadLimit(0)

	w := newWebsocketWriter(conn)
	ctx = mustParseAuthorization(ctx, w, s, r.Authorization, true)

	inst, connIO, err := s.Server.InstanceConnection(ctx, instID)
	if err != nil {
		respondServerError(ctx, w, s, "", "", "", instID, err)
		conn.WriteMessage(websocket.CloseMessage, websocketNormalClosure)
		return
	}

	reply := &api.IOConnection{Connected: connIO != nil}
	err = conn.WriteMessage(websocket.TextMessage, jsonproto.MustMarshal(reply))
	if err != nil {
		reportNetworkError(ctx, s, err)
		return
	}

	if connIO == nil {
		conn.WriteMessage(websocket.CloseMessage, websocketNotConnected)
		return
	}

	err = connIO(ctx, newWebsocketReader(conn), newWebsocketWriter(conn))
	if err != nil {
		// Network error has already been reported by connIO.
		return
	}

	data, err := jsonproto.Marshal(&api.ConnectionStatus{Status: inst.Status()})
	if err != nil {
		conn.WriteMessage(websocket.CloseMessage, websocketInternalServerErr)
		reportInternalError(ctx, s, "", "", "", "", err)
		return
	}

	err = conn.WriteMessage(websocket.TextMessage, data)
	if err != nil {
		reportNetworkError(ctx, s, err)
		return
	}

	conn.WriteMessage(websocket.CloseMessage, websocketNormalClosure)
}

func handleInstanceSnapshot(w http.ResponseWriter, r *http.Request, s *webserver, instID string) {
	ctx := server.ContextWithOp(r.Context(), server.OpInstanceSnapshot)
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	moduleKey, err := s.Server.InstanceModule(ctx, instID)
	if err != nil {
		respondServerError(ctx, wr, s, "", "", "", instID, err)
		return
	}

	w.Header().Set(webapi.HeaderLocation, s.pathModuleRefs+moduleKey)
	w.WriteHeader(http.StatusCreated)
}

func handleInstanceDebug(w http.ResponseWriter, r *http.Request, s *webserver, instID string) {
	mustHaveContentType(w, r, s, webapi.ContentTypeJSON)
	ctx := server.ContextWithOp(r.Context(), server.OpInstanceDebug)
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	var req serverapi.DebugRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondContentParseError(ctx, wr, s, err)
		return
	}

	res, err := s.Server.DebugInstance(ctx, instID, req)
	if err != nil {
		respondServerError(ctx, wr, s, "", "", "", instID, err)
		return
	}

	resContent, err := json.Marshal(res)
	if err != nil {
		panic(err)
	}

	w.Header().Set(webapi.HeaderContentLength, strconv.Itoa(len(resContent)))
	w.Header().Set(webapi.HeaderContentType, webapi.ContentTypeJSON+"; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(resContent)
}
