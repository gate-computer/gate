// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package web

import (
	"context"
	"errors"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"

	server "gate.computer/gate/server/api"
	"gate.computer/gate/server/event"
	"gate.computer/gate/server/internal/monitor"
	"gate.computer/gate/server/web/api"
	internalapi "gate.computer/gate/server/web/internal/api"
	"gate.computer/gate/server/web/internal/protojson"
	"gate.computer/internal/principal"
	"github.com/gorilla/websocket"
)

const maxWebsocketRequestSize = 4096

type respondedType struct{}

var responded respondedType

type errorWriter interface {
	SetHeader(key, value string)
	WriteError(status int, text string)
}

type (
	instanceMethod       func(ctx context.Context, s server.Server, instance string) error
	instanceStatusMethod func(ctx context.Context, s server.Server, instance string) (*server.Status, error)
	instanceWaiterMethod func(ctx context.Context, s server.Server, instance string) (server.Instance, error)
)

func deleteInstance(ctx context.Context, s server.Server, instance string) error {
	return s.DeleteInstance(ctx, instance)
}

func killInstance(ctx context.Context, s server.Server, instance string) (server.Instance, error) {
	return s.KillInstance(ctx, instance)
}

func suspendInstance(ctx context.Context, s server.Server, instance string) (server.Instance, error) {
	return s.SuspendInstance(ctx, instance)
}

func waitInstance(ctx context.Context, s server.Server, instance string) (*server.Status, error) {
	return s.WaitInstance(ctx, instance)
}

type privateConfig struct {
	Config
}

type webserver struct {
	privateConfig
	identity           string // JWT audience.
	pathKnownModules   string
	anyOrigin          bool
	localAuthorization bool
}

func NewHandler(pattern string, config *Config) http.Handler {
	return newHandler(pattern, config, "https", false)
}

// NewHandlerWithUnsecuredLocalAuthorization processes requests with unsigned
// JWT tokens under the local principal's identity.  Such tokens can be created
// by anyone without any secret knowledge.
func NewHandlerWithUnsecuredLocalAuthorization(pattern string, config *Config) http.Handler {
	for _, origin := range config.Origins {
		if strings.Contains(origin, "*") {
			panic("origin check disabled for unsecured local handler")
		}
	}

	if config.NonceStorage != nil {
		panic("nonce storage not supported for local principal")
	}

	return newHandler(pattern, config, "http", true)
}

func newHandler(pattern string, config *Config, scheme string, localAuthorization bool) http.Handler {
	s := &webserver{
		localAuthorization: localAuthorization,
	}

	if config != nil {
		s.Config = *config
	}
	if s.Authority == "" {
		s.Authority = strings.SplitN(pattern, "/", 2)[0]
	}
	if s.NewRequestID == nil {
		s.NewRequestID = defaultNewRequestID
	}
	if s.Monitor == nil {
		s.Monitor = monitor.Default
	}
	if !s.Configured() {
		panic("incomplete webserver configuration")
	}

	features := s.Server.Features()

	configOrigins := s.Origins
	s.Origins = nil
	for _, origin := range configOrigins {
		switch origin = strings.TrimSpace(origin); origin {
		case "":
		case "*":
			s.anyOrigin = true
		default:
			s.Origins = append(s.Origins, origin)
		}
	}

	p := strings.TrimRight(pattern, "/")                                    // host/path
	patternAPI := p + api.Path                                              // host/path/api/
	patternModule := p + api.PathModule                                     // host/path/api/module
	patternModules := p + api.PathModuleSources                             // host/path/api/module/
	patternKnownModule := p + api.PathModuleSources + api.KnownModuleSource // host/path/api/module/hash
	patternKnownModules := p + api.PathKnownModules                         // host/path/api/module/hash/
	patternInstances := p + api.PathInstances                               // host/path/api/instance/
	patternInstance := patternInstances[:len(patternInstances)-1]           // host/path/api/instance

	p = strings.TrimLeftFunc(p, func(r rune) bool { return r != '/' })   // /path
	pathAPI := p + api.Path                                              // /path/api/
	pathModule := p + api.PathModule                                     // /path/api/module
	pathModules := p + api.PathModuleSources                             // /path/api/module/
	pathKnownModule := p + api.PathModuleSources + api.KnownModuleSource // /path/api/module/hash
	s.pathKnownModules = p + api.PathKnownModules                        // /path/api/module/hash/
	pathInstances := p + api.PathInstances                               // /path/api/instance/
	pathInstance := pathInstances[:len(pathInstances)-1]                 // /path/api/instance

	s.identity = scheme + "://" + s.Authority + p + api.Path // https://authority/path/api/

	mux := http.NewServeMux()
	mux.HandleFunc(patternAPI, newFeatureHandler(s, pathAPI, features))
	mux.HandleFunc(patternModule, newRedirectHandler(s, pathModule))
	mux.HandleFunc(patternInstance, newRedirectHandler(s, pathInstance))
	mux.HandleFunc(patternInstances, newInstanceHandler(s, pathInstances))
	mux.HandleFunc(patternKnownModule, newRedirectHandler(s, pathKnownModule))
	mux.HandleFunc(patternKnownModules, newKnownModuleHandler(s))

	moduleSources := []string{api.KnownModuleSource}

	for _, relURI := range features.ModuleSources {
		patternSource := patternModule + relURI // host/path/api/module/source
		patternSourceDir := patternSource + "/" // host/path/api/module/source/

		pathSource := pathModule + relURI // /path/api/module/source
		pathSourceDir := pathSource + "/" // /path/api/module/source/

		mux.HandleFunc(patternSource, newRedirectHandler(s, pathSource))
		mux.HandleFunc(patternSourceDir, newModuleSourceHandler(s, pathModule, pathSourceDir))

		moduleSources = append(moduleSources, strings.TrimLeft(relURI, "/"))
	}

	sort.Strings(moduleSources)
	mux.HandleFunc(patternModules, newStaticHandler(s, pathModules, moduleSources))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		ctx = server.ContextWithRequest(ctx, s.NewRequestID(r))
		ctx = server.ContextWithAddress(ctx, r.RemoteAddr)
		r = r.WithContext(ctx)

		s.Monitor(&event.Event{
			Type: event.TypeIfaceAccess,
			Meta: server.ContextMeta(ctx),
		}, nil)

		defer func() {
			if x := recover(); x != nil && x != responded {
				panic(x)
			}
		}()

		if !s.anyOrigin {
			if origin := r.Header.Get(api.HeaderOrigin); origin != "" {
				mustBeAllowedOrigin(w, r, s, origin)
			}
		}

		mux.ServeHTTP(w, r)
	})
}

type staticContent struct {
	content       []byte
	contentLength string
}

func prepareStaticContent(data any) staticContent {
	content := mustMarshalJSON(data)
	return staticContent{
		content:       content,
		contentLength: strconv.Itoa(len(content)),
	}
}

// Path handlers.  Route methods and set up CORS.

func newRedirectHandler(s *webserver, path string) http.HandlerFunc {
	location := path + "/"

	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			respondPathNotFound(w, r, s)
			return
		}

		methods := "OPTIONS"
		setAccessControl(w, r, s, "GET, HEAD, "+methods)

		switch r.Method {
		case "GET", "HEAD":
			w.Header().Set("Location", location)
			w.WriteHeader(http.StatusMovedPermanently)

		case "OPTIONS":
			setOptions(w, methods)

		default:
			respondMethodNotAllowed(w, r, s, methods)
		}
	}
}

func newStaticHandler(s *webserver, path string, data any) http.HandlerFunc {
	static := prepareStaticContent(data)

	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			respondPathNotFound(w, r, s)
			return
		}

		methods := "OPTIONS"
		setAccessControl(w, r, s, "GET, HEAD, "+methods)

		switch r.Method {
		case "GET", "HEAD":
			handleGetStatic(w, r, s, &static)

		case "OPTIONS":
			setOptions(w, methods)

		default:
			respondMethodNotAllowed(w, r, s, methods)
		}
	}
}

func newFeatureHandler(s *webserver, path string, featureAll *server.Features) http.HandlerFunc {
	featureScope := &api.Features{
		Scope: featureAll.Scope,
	}

	answers := [3]staticContent{
		prepareStaticContent(struct{}{}),
		prepareStaticContent(featureScope), // scope
		prepareStaticContent(featureScope), // all
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			respondPathNotFound(w, r, s)
			return
		}

		methods := "OPTIONS"
		setAccessControl(w, r, s, "GET, HEAD, "+methods)

		switch r.Method {
		case "GET", "HEAD":
			handleGetFeatures(w, r, s, &answers)

		case "OPTIONS":
			setOptions(w, methods)

		default:
			respondMethodNotAllowed(w, r, s, methods)
		}
	}
}

func newKnownModuleHandler(s *webserver) http.HandlerFunc {
	var (
		headersList = join(api.HeaderAuthorization)
		headersID   = join(api.HeaderAuthorization, api.HeaderContentType)
		exposed     = join(api.HeaderLocation, api.HeaderInstance, api.HeaderStatus)
	)

	return func(w http.ResponseWriter, r *http.Request) {
		if len(r.URL.Path) == len(s.pathKnownModules) {
			// Module directory listing

			methods := "OPTIONS, POST"
			setAccessControlAllowHeaders(w, r, s, "GET, HEAD, "+methods, headersList)

			switch r.Method {
			case "GET", "HEAD":
				w.WriteHeader(http.StatusNoContent)

			case "OPTIONS":
				setOptions(w, methods)

			case "POST":
				handlePostKnownModules(w, r, s)

			default:
				respondMethodNotAllowed(w, r, s, methods)
			}
		} else {
			// Module operations
			module := r.URL.Path[len(s.pathKnownModules):]

			methods := "OPTIONS, POST, PUT"
			setAccessControlAllowExposeHeaders(w, r, s, "GET, HEAD, "+methods, headersID, exposed)

			switch r.Method {
			case "GET", "HEAD":
				handleGetKnownModule(w, r, s, module)

			case "PUT":
				handlePutKnownModule(w, r, s, module)

			case "POST":
				handlePostKnownModule(w, r, s, module)

			case "OPTIONS":
				setOptions(w, methods)

			default:
				respondMethodNotAllowed(w, r, s, methods)
			}
		}
	}
}

func newModuleSourceHandler(s *webserver, sourceURIBase, sourcePath string) http.HandlerFunc {
	var (
		headers = join(api.HeaderAuthorization)
		exposed = join(api.HeaderLocation, api.HeaderInstance, api.HeaderStatus)
	)

	return func(w http.ResponseWriter, r *http.Request) {
		if len(r.URL.Path) == len(sourcePath) {
			// Module directory listing is not supported for sources.  The
			// directory clearly exists (it has modules in it), but doesn't
			// support any methods itself.

			methods := "OPTIONS"
			setAccessControl(w, r, s, "GET, HEAD, "+methods)

			switch r.Method {
			case "GET", "HEAD":
				w.WriteHeader(http.StatusNoContent)

			case "OPTIONS":
				setOptions(w, methods)

			default:
				respondMethodNotAllowed(w, r, s, methods)
			}
		} else {
			// Module operations
			module := r.URL.Path[len(sourceURIBase):]

			methods := "OPTIONS, POST"
			setAccessControlAllowExposeHeaders(w, r, s, "GET, HEAD, "+methods, headers, exposed)

			switch r.Method {
			case "GET", "HEAD":
				handleGetModuleSource(w, r, s, module)

			case "POST":
				handlePostModuleSource(w, r, s, module)

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
		headersGet  = join(api.HeaderAuthorization)
		headersPost = join(api.HeaderAuthorization, api.HeaderContentType)
		exposed     = join(api.HeaderStatus)
	)

	return func(w http.ResponseWriter, r *http.Request) {
		if len(r.URL.Path) == len(instancesPath) {
			// Instance directory listing

			methods := "OPTIONS"
			setAccessControlAllowHeaders(w, r, s, "GET, HEAD, "+methods, headersGet)

			switch r.Method {
			case "GET", "HEAD":
				w.WriteHeader(http.StatusNoContent)

			case "POST":
				handlePostInstances(w, r, s)

			case "OPTIONS":
				setOptions(w, methods)

			default:
				respondMethodNotAllowed(w, r, s, methods)
			}
		} else {
			// Instance operations
			instance := r.URL.Path[len(instancesPath):]

			methods := "OPTIONS, POST"
			setAccessControlAllowExposeHeaders(w, r, s, "GET, HEAD, "+methods, headersPost, exposed)

			switch r.Method {
			case "GET", "HEAD":
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

func handleGetStatic(w http.ResponseWriter, r *http.Request, s *webserver, static *staticContent) {
	mustNotHaveQuery(w, r, s)
	mustAcceptJSON(w, r, s)
	handleStatic(w, r, s, static)
}

func handleGetFeatures(w http.ResponseWriter, r *http.Request, s *webserver, answers *[3]staticContent) {
	query := mustParseOptionalQuery(w, r, s)
	features := popOptionalParams(query, api.ParamFeature)
	mustNotHaveParams(w, r, s, query)

	if len(features) > 0 {
		mustAcceptJSON(w, r, s)
	}

	level := 0

	for _, v := range features {
		switch v {
		case api.FeatureScope:
			if level == 0 {
				level = 1
			}

		case api.FeatureAll:
			level = len(*answers) - 1

		default:
			respondUnsupportedFeature(w, r, s)
			return
		}
	}

	handleStatic(w, r, s, &(*answers)[level])
}

func handleGetKnownModule(w http.ResponseWriter, r *http.Request, s *webserver, key string) {
	query := mustParseOptionalQuery(w, r, s)
	pin := popOptionalActionParam(w, r, s, query, api.ActionPin)

	var modTags []string
	if pin {
		modTags = popOptionalParams(query, api.ParamModuleTag)
	}

	if _, found := query[api.ParamAction]; found {
		switch popLastParam(w, r, s, query, api.ParamAction) {
		case api.ActionCall:
			function := mustPopOptionalLastFunctionParam(w, r, s, query)
			instTags := popOptionalParams(query, api.ParamInstanceTag)
			invoke := popOptionalLastLogParam(w, r, s, query)
			mustNotHaveParams(w, r, s, query)
			handleCallWebsocket(w, r, s, pin, "", key, function, modTags, instTags, invoke)

		default:
			respondUnsupportedAction(w, r, s)
		}
	} else {
		if pin {
			respondUnsupportedAction(w, r, s)
		} else {
			mustNotHaveParams(w, r, s, query)
			mustAcceptWebAssembly(w, r, s)
			handleModuleDownload(w, r, s, key)
		}
	}
}

func handlePostKnownModules(w http.ResponseWriter, r *http.Request, s *webserver) {
	mustNotHaveQuery(w, r, s)
	mustNotHaveContentType(w, r, s)
	mustNotHaveContent(w, r, s)
	mustAcceptJSON(w, r, s)
	handleModuleList(w, r, s)
}

func handlePutKnownModule(w http.ResponseWriter, r *http.Request, s *webserver, key string) {
	mustHaveContentType(w, r, s, api.ContentTypeWebAssembly)
	mustHaveContentLength(w, r, s)
	query := mustParseOptionalQuery(w, r, s)
	pin := popOptionalActionParam(w, r, s, query, api.ActionPin)
	suspend := popOptionalActionParam(w, r, s, query, api.ActionSuspend)

	var modTags []string
	if pin {
		modTags = popOptionalParams(query, api.ParamModuleTag)
	}

	if _, found := query[api.ParamAction]; found {
		switch popLastParam(w, r, s, query, api.ParamAction) {
		case api.ActionCall:
			function := mustPopOptionalLastFunctionParam(w, r, s, query)
			instTags := popOptionalParams(query, api.ParamInstanceTag)
			invoke := popOptionalLastLogParam(w, r, s, query)
			mustNotHaveParams(w, r, s, query)
			handleCall(w, r, s, server.OpCallUpload, pin, true, "", key, function, modTags, instTags, invoke)

		case api.ActionLaunch:
			function := mustPopOptionalLastFunctionParam(w, r, s, query)
			instance := popOptionalLastParam(w, r, s, query, api.ParamInstance)
			instTags := popOptionalParams(query, api.ParamInstanceTag)
			invoke := popOptionalLastLogParam(w, r, s, query)
			mustNotHaveParams(w, r, s, query)
			handleLaunchUpload(w, r, s, pin, key, function, instance, modTags, instTags, suspend, invoke)

		default:
			respondUnsupportedAction(w, r, s)
		}
	} else {
		if pin {
			mustNotHaveParams(w, r, s, query)
			handleModuleUpload(w, r, s, key, modTags)
		} else {
			respondUnsupportedAction(w, r, s)
		}
	}
}

func handlePostKnownModule(w http.ResponseWriter, r *http.Request, s *webserver, key string) {
	query := mustParseQuery(w, r, s)

	if len(query[api.ParamAction]) == 0 {
		mustNotHaveParams(w, r, s, query)
		mustAcceptJSON(w, r, s)
		handleKnownModule(w, r, s, key)
		return
	}

	suspend := popOptionalActionParam(w, r, s, query, api.ActionSuspend)

	switch popLastParam(w, r, s, query, api.ParamAction) {
	case api.ActionCall:
		function := mustPopOptionalLastFunctionParam(w, r, s, query)
		instTags := popOptionalParams(query, api.ParamInstanceTag)
		invoke := popOptionalLastLogParam(w, r, s, query)
		mustNotHaveParams(w, r, s, query)
		handleCall(w, r, s, server.OpCallExtant, false, false, "", key, function, nil, instTags, invoke)

	case api.ActionLaunch:
		function := mustPopOptionalLastFunctionParam(w, r, s, query)
		instance := popOptionalLastParam(w, r, s, query, api.ParamInstance)
		instTags := popOptionalParams(query, api.ParamInstanceTag)
		invoke := popOptionalLastLogParam(w, r, s, query)
		mustNotHaveParams(w, r, s, query)
		mustNotHaveContentType(w, r, s)
		mustNotHaveContent(w, r, s)
		handleLaunch(w, r, s, server.OpLaunchExtant, false, "", key, function, instance, nil, instTags, suspend, invoke)

	case api.ActionPin:
		modTags := popOptionalParams(query, api.ParamModuleTag)
		mustNotHaveParams(w, r, s, query)
		mustNotHaveContentType(w, r, s)
		mustNotHaveContent(w, r, s)
		handleModulePin(w, r, s, key, modTags)

	case api.ActionUnpin:
		mustNotHaveParams(w, r, s, query)
		mustNotHaveContentType(w, r, s)
		mustNotHaveContent(w, r, s)
		handleModuleUnpin(w, r, s, key)

	default:
		respondUnsupportedAction(w, r, s)
	}
}

func handleGetModuleSource(w http.ResponseWriter, r *http.Request, s *webserver, source string) {
	query := mustParseQuery(w, r, s)
	pin := popOptionalActionParam(w, r, s, query, api.ActionPin)

	var modTags []string
	if pin {
		modTags = popOptionalParams(query, api.ParamModuleTag)
	}

	switch popLastParam(w, r, s, query, api.ParamAction) {
	case api.ActionCall:
		function := mustPopOptionalLastFunctionParam(w, r, s, query)
		instTags := popOptionalParams(query, api.ParamInstanceTag)
		invoke := popOptionalLastLogParam(w, r, s, query)
		mustNotHaveParams(w, r, s, query)
		handleCallWebsocket(w, r, s, pin, source, "", function, modTags, instTags, invoke)

	default:
		respondUnsupportedAction(w, r, s)
	}
}

func handlePostModuleSource(w http.ResponseWriter, r *http.Request, s *webserver, source string) {
	query := mustParseQuery(w, r, s)
	pin := popOptionalActionParam(w, r, s, query, api.ActionPin)

	var modTags []string
	if pin {
		modTags = popOptionalParams(query, api.ParamModuleTag)
	}

	if _, found := query[api.ParamAction]; found {
		suspend := popOptionalActionParam(w, r, s, query, api.ActionSuspend)

		switch popLastParam(w, r, s, query, api.ParamAction) {
		case api.ActionCall:
			if suspend {
				respondUnsupportedAction(w, r, s)
			}
			function := mustPopOptionalLastFunctionParam(w, r, s, query)
			instTags := popOptionalParams(query, api.ParamInstanceTag)
			invoke := popOptionalLastLogParam(w, r, s, query)
			mustNotHaveParams(w, r, s, query)
			handleCall(w, r, s, server.OpCallSource, pin, false, source, "", function, modTags, instTags, invoke)

		case api.ActionLaunch:
			function := mustPopOptionalLastFunctionParam(w, r, s, query)
			instance := popOptionalLastParam(w, r, s, query, api.ParamInstance)
			instTags := popOptionalParams(query, api.ParamInstanceTag)
			invoke := popOptionalLastLogParam(w, r, s, query)
			mustNotHaveParams(w, r, s, query)
			mustNotHaveContentType(w, r, s)
			mustNotHaveContent(w, r, s)
			handleLaunch(w, r, s, server.OpLaunchSource, pin, source, "", function, instance, modTags, instTags, suspend, invoke)

		default:
			respondUnsupportedAction(w, r, s)
		}
	} else {
		if pin {
			mustNotHaveParams(w, r, s, query)
			mustNotHaveContentType(w, r, s)
			mustNotHaveContent(w, r, s)
			handleModuleSource(w, r, s, source, modTags)
		} else {
			respondUnsupportedAction(w, r, s)
		}
	}
}

func handleGetInstance(w http.ResponseWriter, r *http.Request, s *webserver, instance string) {
	query := mustParseQuery(w, r, s)

	switch popLastParam(w, r, s, query, api.ParamAction) {
	case api.ActionIO:
		mustNotHaveParams(w, r, s, query)
		handleInstanceConnectWebsocket(w, r, s, instance)

	default:
		respondUnsupportedAction(w, r, s)
	}
}

func handlePostInstances(w http.ResponseWriter, r *http.Request, s *webserver) {
	mustNotHaveQuery(w, r, s)
	mustNotHaveContentType(w, r, s)
	mustNotHaveContent(w, r, s)
	mustAcceptJSON(w, r, s)
	handleInstanceList(w, r, s)
}

func handlePostInstance(w http.ResponseWriter, r *http.Request, s *webserver, instance string) {
	query := mustParseQuery(w, r, s)
	actions := popOptionalParams(query, api.ParamAction)

	switch len(actions) {
	case 0:
		mustNotHaveParams(w, r, s, query)
		mustAcceptJSON(w, r, s)
		handleInstanceInfo(w, r, s, instance)
		return

	case 1:
		switch actions[0] {
		case api.ActionIO:
			mustNotHaveParams(w, r, s, query)
			handleInstanceConnect(w, r, s, instance)
			return

		case api.ActionResume:
			function := mustPopOptionalLastFunctionParam(w, r, s, query)
			invoke := popOptionalLastLogParam(w, r, s, query)
			mustNotHaveParams(w, r, s, query)
			handleInstanceResume(w, r, s, function, instance, invoke)
			return

		case api.ActionSnapshot:
			modTags := popOptionalParams(query, api.ParamModuleTag)
			mustNotHaveParams(w, r, s, query)
			handleInstanceSnapshot(w, r, s, instance, modTags)
			return

		case api.ActionDelete:
			mustNotHaveParams(w, r, s, query)
			handleInstance(w, r, s, server.OpInstanceDelete, deleteInstance, instance)
			return

		case api.ActionUpdate:
			mustNotHaveParams(w, r, s, query)
			handleInstanceUpdate(w, r, s, instance)
			return

		case api.ActionDebug:
			mustNotHaveParams(w, r, s, query)
			handleInstanceDebug(w, r, s, instance)
			return
		}
	}

	var (
		kill    bool
		suspend bool
		wait    bool
	)
	for _, a := range actions {
		switch a {
		case api.ActionKill:
			kill = true

		case api.ActionSuspend:
			suspend = true

		case api.ActionWait:
			wait = true

		default:
			respondUnsupportedAction(w, r, s)
			return
		}
	}

	switch {
	case kill && !suspend:
		mustNotHaveParams(w, r, s, query)
		handleInstanceWaiter(w, r, s, server.OpInstanceKill, killInstance, instance, wait)

	case suspend && !kill:
		mustNotHaveParams(w, r, s, query)
		handleInstanceWaiter(w, r, s, server.OpInstanceSuspend, suspendInstance, instance, wait)

	case wait && !kill && !suspend:
		mustNotHaveParams(w, r, s, query)
		handleInstanceStatus(w, r, s, server.OpInstanceWait, waitInstance, instance)

	default:
		respondUnsupportedAction(w, r, s)
	}
}

// Action handlers.  Check authorization if needed, and serve the response.

func handleStatic(w http.ResponseWriter, r *http.Request, s *webserver, static *staticContent) {
	w.Header().Set("Cache-Control", cacheControlStatic)
	w.Header().Set(api.HeaderContentLength, static.contentLength)
	w.Header().Set(api.HeaderContentType, contentTypeJSON)
	w.WriteHeader(http.StatusOK)
	w.Write(static.content)
}

func handleModuleList(w http.ResponseWriter, r *http.Request, s *webserver) {
	ctx := server.ContextWithOp(r.Context(), server.OpModuleList)
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	infos, err := s.Server.Modules(ctx)
	if err != nil {
		respondServerError(ctx, wr, s, "", "", "", "", err)
		return
	}

	sort.Sort(server.SortableModules(infos))
	content := protojson.MustMarshal(infos)
	w.Header().Set(api.HeaderContentLength, strconv.Itoa(len(content)))
	w.Header().Set(api.HeaderContentType, contentTypeJSON)
	w.WriteHeader(http.StatusOK)
	w.Write(content)
}

func handleKnownModule(w http.ResponseWriter, r *http.Request, s *webserver, key string) {
	ctx := server.ContextWithOp(r.Context(), server.OpModuleInfo)
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	info, err := s.Server.ModuleInfo(ctx, key)
	if err != nil {
		respondServerError(ctx, wr, s, "", key, "", "", err)
		return
	}

	content := protojson.MustMarshal(info)
	w.Header().Set(api.HeaderContentLength, strconv.Itoa(len(content)))
	w.Header().Set(api.HeaderContentType, contentTypeJSON)
	w.WriteHeader(http.StatusOK)
	w.Write(content)
}

func handleModuleDownload(w http.ResponseWriter, r *http.Request, s *webserver, key string) {
	ctx := server.ContextWithOp(r.Context(), server.OpModuleDownload)
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	content, length, err := s.Server.ModuleContent(ctx, key)
	if err != nil {
		respondServerError(ctx, wr, s, "", key, "", "", err)
		panic(responded)
	}
	defer content.Close()

	w.Header().Set(api.HeaderContentLength, strconv.FormatInt(length, 10))
	w.Header().Set(api.HeaderContentType, api.ContentTypeWebAssembly)
	w.WriteHeader(http.StatusOK)

	if r.Method != "HEAD" {
		io.Copy(w, content)
	}
}

func handleModuleUpload(w http.ResponseWriter, r *http.Request, s *webserver, key string, modTags []string) {
	ctx := server.ContextWithOp(r.Context(), server.OpModuleUpload)
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)
	upload := moduleUpload(mustDecodeContent(ctx, wr, s), r.ContentLength, key)
	defer upload.Close()

	if _, err := s.Server.UploadModule(ctx, upload, modulePin(true, modTags)); err != nil {
		respondServerError(ctx, wr, s, "", key, "", "", err)
		panic(responded)
	}

	w.WriteHeader(http.StatusCreated)
}

func handleModuleSource(w http.ResponseWriter, r *http.Request, s *webserver, source string, modTags []string) {
	ctx := server.ContextWithOp(r.Context(), server.OpModuleSource)
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	module, err := s.Server.SourceModule(ctx, source, modulePin(true, modTags))
	if err != nil {
		respondServerError(ctx, wr, s, source, "", "", "", err)
		return
	}

	w.Header().Set(api.HeaderLocation, s.pathKnownModules+module)
	w.WriteHeader(http.StatusCreated)
}

func handleModulePin(w http.ResponseWriter, r *http.Request, s *webserver, key string, modTags []string) {
	ctx := server.ContextWithOp(r.Context(), server.OpModulePin)
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	if err := s.Server.PinModule(ctx, key, modulePin(true, modTags)); err != nil {
		respondServerError(ctx, wr, s, "", key, "", "", err)
		panic(responded)
	}

	w.WriteHeader(http.StatusNoContent)
}

func handleModuleUnpin(w http.ResponseWriter, r *http.Request, s *webserver, key string) {
	ctx := server.ContextWithOp(r.Context(), server.OpModuleUnpin)
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	if err := s.Server.UnpinModule(ctx, key); err != nil {
		respondServerError(ctx, wr, s, "", key, "", "", err)
		panic(responded)
	}

	w.WriteHeader(http.StatusNoContent)
}

func handleCall(w http.ResponseWriter, r *http.Request, s *webserver, op server.Op, pin, content bool, source, key, function string, modTags, instTags []string, invoke *server.InvokeOptions) {
	ctx := server.ContextWithOp(r.Context(), op) // TODO: detail: post
	wr := &requestResponseWriter{w, r}

	launch := &server.LaunchOptions{
		Invoke:    invoke,
		Function:  function,
		Transient: true,
		Tags:      instTags,
	}

	var (
		module string
		inst   server.Instance
		err    error
	)
	switch {
	case content:
		ctx = mustParseAuthorizationHeader(ctx, wr, s, pin)
		upload := moduleUpload(mustDecodeContent(ctx, wr, s), r.ContentLength, key)
		defer upload.Close()

		module, inst, err = s.Server.UploadModuleInstance(ctx, upload, modulePin(pin, modTags), launch)
		if err != nil {
			respondServerError(ctx, wr, s, "", key, function, "", err)
			return
		}

	case source == "":
		ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

		module = key
		inst, err = s.Server.NewInstance(ctx, key, launch)
		if err != nil {
			respondServerError(ctx, wr, s, "", key, function, "", err)
			return
		}

	default:
		ctx = mustParseAuthorizationHeader(ctx, wr, s, pin)

		module, inst, err = s.Server.SourceModuleInstance(ctx, source, modulePin(pin, modTags), launch)
		if err != nil {
			// TODO: find out module hash
			respondServerError(ctx, wr, s, source, "", function, "", err)
			return
		}
	}
	defer inst.Kill(ctx) // TODO: error

	trail := acceptsTrailers(r)
	if trail {
		w.Header().Set("Trailer", api.HeaderStatus)
	}

	if principal.ContextID(ctx) != nil {
		if pin {
			w.Header().Set(api.HeaderLocation, s.pathKnownModules+module)
		}
		w.Header().Set(api.HeaderInstance, inst.ID())
	}

	if pin {
		w.WriteHeader(http.StatusCreated)
	} else {
		w.WriteHeader(http.StatusOK)
	}

	inst.Connect(ctx, r.Body, nopCloser{w})
	status := inst.Wait(ctx)

	if trail {
		w.Header().Set(api.HeaderStatus, string(protojson.MustMarshal(status)))
	}
}

func handleCallWebsocket(response http.ResponseWriter, request *http.Request, s *webserver, pin bool, source, key, function string, modTags, instTags []string, invoke *server.InvokeOptions) {
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

	var r api.Call

	err = conn.ReadJSON(&r)
	if err != nil {
		if e := net.Error(nil); errors.As(err, &e) {
			reportNetworkError(ctx, s, err)
		} else {
			reportProtocolError(ctx, s, err)
		}
		return
	}

	conn.SetReadLimit(0)

	var content bool

	switch r.ContentType {
	case api.ContentTypeWebAssembly:
		if source != "" {
			conn.WriteMessage(websocket.CloseMessage, websocketUnsupportedContent)
			reportProtocolError(ctx, s, errUnsupportedWebsocketContent)
			return
		}

		ctx = server.ContextWithOp(ctx, server.OpCallUpload) // TODO: detail: websocket
		content = true

	case "":
		if source == "" {
			ctx = server.ContextWithOp(ctx, server.OpCallExtant) // TODO: detail: websocket
		} else {
			ctx = server.ContextWithOp(ctx, server.OpCallSource) // TODO: detail: websocket
		}

	default:
		conn.WriteMessage(websocket.CloseMessage, websocketUnsupportedContentType)
		reportProtocolError(ctx, s, errUnsupportedWebsocketContentType)
		return
	}

	w := websocketResponseWriter{conn}

	launch := &server.LaunchOptions{
		Invoke:    invoke,
		Function:  function,
		Transient: true,
		Tags:      instTags,
	}

	var (
		module string
		inst   server.Instance
	)
	switch {
	case content:
		ctx = mustParseAuthorization(ctx, w, s, r.Authorization, pin)

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
		upload := moduleUpload(ioutil.NopCloser(frame), r.ContentLength, key)
		defer upload.Close()

		module, inst, err = s.Server.UploadModuleInstance(ctx, upload, modulePin(pin, modTags), launch)
		if err != nil {
			respondServerError(ctx, w, s, "", key, function, "", err)
			return
		}

	case source == "":
		ctx = mustParseAuthorization(ctx, w, s, r.Authorization, false)

		module = key
		inst, err = s.Server.NewInstance(ctx, key, launch)
		if err != nil {
			respondServerError(ctx, w, s, "", key, function, "", err)
			return
		}

	default:
		ctx = mustParseAuthorization(ctx, w, s, r.Authorization, pin)

		module, inst, err = s.Server.SourceModuleInstance(ctx, source, modulePin(pin, modTags), launch)
		if err != nil {
			// TODO: find out module hash
			respondServerError(ctx, w, s, source, "", function, "", err)
			return
		}
	}
	defer inst.Kill(ctx) // TODO: error

	var reply api.CallConnection

	if principal.ContextID(ctx) != nil {
		if pin {
			reply.Location = s.pathKnownModules + module
		}
		reply.Instance = inst.ID()
	}

	err = conn.WriteJSON(&reply)
	if err != nil {
		reportNetworkError(ctx, s, err)
		return
	}

	rw := newWebsocketReadWriteCanceler(conn, cancel)
	inst.Connect(ctx, rw, rw)
	status := inst.Wait(ctx)
	data := protojson.MustMarshal(&internalapi.ConnectionStatus{Status: status})
	if conn.WriteMessage(websocket.TextMessage, data) == nil {
		conn.WriteMessage(websocket.CloseMessage, websocketNormalClosure)
	}
}

func handleLaunch(w http.ResponseWriter, r *http.Request, s *webserver, op server.Op, pin bool, source, key, function, instance string, modTags, instTags []string, suspend bool, invoke *server.InvokeOptions) {
	ctx := server.ContextWithOp(r.Context(), op)
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	launch := &server.LaunchOptions{
		Invoke:   invoke,
		Function: function,
		Instance: instance,
		Suspend:  suspend,
		Tags:     instTags,
	}

	var (
		module string
		inst   server.Instance
		err    error
	)
	if source == "" {
		module = key
		inst, err = s.Server.NewInstance(ctx, key, launch)
		if err != nil {
			respondServerError(ctx, wr, s, "", key, function, "", err)
			return
		}
	} else {
		module, inst, err = s.Server.SourceModuleInstance(ctx, source, modulePin(pin, modTags), launch)
		if err != nil {
			respondServerError(ctx, wr, s, source, "", function, "", err)
			return
		}
	}

	w.Header().Set(api.HeaderInstance, inst.ID())

	if pin {
		w.Header().Set(api.HeaderLocation, s.pathKnownModules+module)
		w.WriteHeader(http.StatusCreated)
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleLaunchUpload(w http.ResponseWriter, r *http.Request, s *webserver, pin bool, key, function, instance string, modTags, instTags []string, suspend bool, invoke *server.InvokeOptions) {
	ctx := server.ContextWithOp(r.Context(), server.OpLaunchUpload)
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	launch := &server.LaunchOptions{
		Invoke:   invoke,
		Function: function,
		Instance: instance,
		Suspend:  suspend,
		Tags:     instTags,
	}

	upload := moduleUpload(mustDecodeContent(ctx, wr, s), r.ContentLength, key)
	defer upload.Close()

	key, inst, err := s.Server.UploadModuleInstance(ctx, upload, modulePin(pin, modTags), launch)
	if err != nil {
		respondServerError(ctx, wr, s, "", key, function, "", err)
		return
	}

	w.Header().Set(api.HeaderInstance, inst.ID())

	if pin {
		w.Header().Set(api.HeaderLocation, s.pathKnownModules+key)
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

	sort.Sort(server.SortableInstances(instances))
	content := protojson.MustMarshal(instances)
	w.Header().Set(api.HeaderContentLength, strconv.Itoa(len(content)))
	w.Header().Set(api.HeaderContentType, contentTypeJSON)
	w.WriteHeader(http.StatusOK)
	w.Write(content)
}

func handleInstance(w http.ResponseWriter, r *http.Request, s *webserver, op server.Op, method instanceMethod, instance string) {
	ctx := server.ContextWithOp(r.Context(), op)
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	if err := method(ctx, s.Server, instance); err != nil {
		respondServerError(ctx, wr, s, "", "", "", instance, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func handleInstanceInfo(w http.ResponseWriter, r *http.Request, s *webserver, instance string) {
	ctx := server.ContextWithOp(r.Context(), server.OpInstanceInfo)
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	info, err := s.Server.InstanceInfo(ctx, instance)
	if err != nil {
		respondServerError(ctx, wr, s, "", "", "", instance, err)
		return
	}

	content := protojson.MustMarshal(info)
	w.Header().Set(api.HeaderContentLength, strconv.Itoa(len(content)))
	w.Header().Set(api.HeaderContentType, contentTypeJSON)
	w.WriteHeader(http.StatusOK)
	w.Write(content)
}

func handleInstanceStatus(w http.ResponseWriter, r *http.Request, s *webserver, op server.Op, method instanceStatusMethod, instance string) {
	ctx := server.ContextWithOp(r.Context(), op)
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	status, err := method(ctx, s.Server, instance)
	if err != nil {
		respondServerError(ctx, wr, s, "", "", "", instance, err)
		return
	}

	w.Header().Set(api.HeaderStatus, string(protojson.MustMarshal(status)))
	w.WriteHeader(http.StatusNoContent)
}

func handleInstanceWaiter(w http.ResponseWriter, r *http.Request, s *webserver, op server.Op, method instanceWaiterMethod, instance string, wait bool) {
	ctx := server.ContextWithOp(r.Context(), op)
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	inst, err := method(ctx, s.Server, instance)
	if err != nil {
		respondServerError(ctx, wr, s, "", "", "", instance, err)
		return
	}

	if wait {
		status := inst.Wait(ctx)
		w.Header().Set(api.HeaderStatus, string(protojson.MustMarshal(status)))
	}

	w.WriteHeader(http.StatusNoContent)
}

func handleInstanceResume(w http.ResponseWriter, r *http.Request, s *webserver, function, instance string, invoke *server.InvokeOptions) {
	ctx := server.ContextWithOp(r.Context(), server.OpInstanceResume)
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	resume := &server.ResumeOptions{
		Invoke:   invoke,
		Function: function,
	}

	if _, err := s.Server.ResumeInstance(ctx, instance, resume); err != nil {
		respondServerError(ctx, wr, s, "", "", function, instance, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func handleInstanceConnect(w http.ResponseWriter, r *http.Request, s *webserver, instance string) {
	ctx := server.ContextWithOp(r.Context(), server.OpInstanceConnect) // TODO: detail: post
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	content := mustDecodeContent(ctx, wr, s)
	defer content.Close()

	inst, connIO, err := s.Server.InstanceConnection(ctx, instance)
	if err != nil {
		respondServerError(ctx, wr, s, "", "", "", instance, err)
		return
	}
	if connIO == nil {
		w.WriteHeader(http.StatusConflict)
		return
	}

	trail := acceptsTrailers(r)
	if trail {
		w.Header().Set("Trailer", api.HeaderStatus)
	}

	w.WriteHeader(http.StatusOK)

	if err := connIO(ctx, content, nopCloser{w}); err != nil {
		// Network error has already been reported by connIO.
		return
	}

	status := inst.Status(ctx)

	if trail {
		w.Header().Set(api.HeaderStatus, string(protojson.MustMarshal(status)))
	}
}

func handleInstanceConnectWebsocket(response http.ResponseWriter, request *http.Request, s *webserver, instance string) {
	ctx := server.ContextWithOp(request.Context(), server.OpInstanceConnect) // TODO: detail: websocket

	conn, err := websocketUpgrader.Upgrade(response, request, nil)
	if err != nil {
		reportProtocolError(ctx, s, err)
		return
	}
	defer conn.Close()

	conn.SetReadLimit(maxWebsocketRequestSize)

	var r api.IO

	err = conn.ReadJSON(&r)
	if err != nil {
		if e := net.Error(nil); errors.As(err, &e) {
			reportNetworkError(ctx, s, err)
		} else {
			reportProtocolError(ctx, s, err)
		}
		return
	}

	conn.SetReadLimit(0)

	w := websocketResponseWriter{conn}
	ctx = mustParseAuthorization(ctx, w, s, r.Authorization, true)

	inst, connIO, err := s.Server.InstanceConnection(ctx, instance)
	if err != nil {
		respondServerError(ctx, w, s, "", "", "", instance, err)
		conn.WriteMessage(websocket.CloseMessage, websocketNormalClosure)
		return
	}

	reply := &internalapi.IOConnection{Connected: connIO != nil}
	err = conn.WriteMessage(websocket.TextMessage, protojson.MustMarshal(reply))
	if err != nil {
		reportNetworkError(ctx, s, err)
		return
	}

	if connIO == nil {
		conn.WriteMessage(websocket.CloseMessage, websocketNotConnected)
		return
	}

	rw := newWebsocketReadWriter(conn)
	err = connIO(ctx, rw, rw)
	if err != nil {
		// Network error has already been reported by connIO.
		return
	}

	status := inst.Status(ctx)
	data := protojson.MustMarshal(&internalapi.ConnectionStatus{Status: status})
	err = conn.WriteMessage(websocket.TextMessage, data)
	if err != nil {
		reportNetworkError(ctx, s, err)
		return
	}

	conn.WriteMessage(websocket.CloseMessage, websocketNormalClosure)
}

func handleInstanceSnapshot(w http.ResponseWriter, r *http.Request, s *webserver, instance string, modTags []string) {
	ctx := server.ContextWithOp(r.Context(), server.OpInstanceSnapshot)
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	module, err := s.Server.Snapshot(ctx, instance, modulePin(true, modTags))
	if err != nil {
		respondServerError(ctx, wr, s, "", "", "", instance, err)
		return
	}

	w.Header().Set(api.HeaderLocation, s.pathKnownModules+module)
	w.WriteHeader(http.StatusCreated)
}

func handleInstanceUpdate(w http.ResponseWriter, r *http.Request, s *webserver, instance string) {
	mustHaveContentType(w, r, s, api.ContentTypeJSON)
	ctx := server.ContextWithOp(r.Context(), server.OpInstanceUpdate)
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	req := new(server.InstanceUpdate)
	if err := protojson.Decode(r.Body, req); err != nil {
		respondContentParseError(ctx, wr, s, err)
		return
	}

	info, err := s.Server.UpdateInstance(ctx, instance, req)
	if err != nil {
		respondServerError(ctx, wr, s, "", "", "", instance, err)
		return
	}

	if !acceptsJSON(r) {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	content := protojson.MustMarshal(info)
	w.Header().Set(api.HeaderContentLength, strconv.Itoa(len(content)))
	w.Header().Set(api.HeaderContentType, contentTypeJSON)
	w.WriteHeader(http.StatusOK)
	w.Write(content)
}

func handleInstanceDebug(w http.ResponseWriter, r *http.Request, s *webserver, instance string) {
	mustHaveContentType(w, r, s, api.ContentTypeJSON)
	ctx := server.ContextWithOp(r.Context(), server.OpInstanceDebug)
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	req := new(server.DebugRequest)
	if err := protojson.Decode(r.Body, req); err != nil {
		respondContentParseError(ctx, wr, s, err)
		return
	}

	res, err := s.Server.DebugInstance(ctx, instance, req)
	if err != nil {
		respondServerError(ctx, wr, s, "", "", "", instance, err)
		return
	}

	content := protojson.MustMarshal(res)
	w.Header().Set(api.HeaderContentLength, strconv.Itoa(len(content)))
	w.Header().Set(api.HeaderContentType, contentTypeJSON)
	w.WriteHeader(http.StatusOK)
	w.Write(content)
}

func moduleUpload(s io.ReadCloser, length int64, hash string) *server.ModuleUpload {
	return &server.ModuleUpload{
		Stream: s,
		Length: length,
		Hash:   hash,
	}
}

func modulePin(pin bool, tags []string) *server.ModuleOptions {
	return &server.ModuleOptions{
		Pin:  pin,
		Tags: tags,
	}
}

type nopCloser struct {
	io.Writer
}

func (nopCloser) Close() error {
	return nil
}
