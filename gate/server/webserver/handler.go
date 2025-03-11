// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webserver

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"gate.computer/gate/server/api"
	"gate.computer/gate/server/logging"
	"gate.computer/gate/web"
	"gate.computer/internal/principal"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/encoding/protojson"

	. "import.name/type/context"
)

const maxWebsocketRequestSize = 4096

type respondedType struct{}

var responded respondedType

type errorWriter interface {
	SetHeader(key, value string)
	WriteError(status int, text string)
}

type (
	instanceMethod       func(ctx Context, s api.Server, instance string) error
	instanceStatusMethod func(ctx Context, s api.Server, instance string) (*api.Status, error)
	instanceWaiterMethod func(ctx Context, s api.Server, instance string) (api.Instance, error)
)

func deleteInstance(ctx Context, s api.Server, instance string) error {
	return s.DeleteInstance(ctx, instance)
}

func killInstance(ctx Context, s api.Server, instance string) (api.Instance, error) {
	return s.KillInstance(ctx, instance)
}

func suspendInstance(ctx Context, s api.Server, instance string) (api.Instance, error) {
	return s.SuspendInstance(ctx, instance)
}

func waitInstance(ctx Context, s api.Server, instance string) (*api.Status, error) {
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

	if config.NonceChecker != nil {
		panic("NonceChecker is not applicable with local principal")
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
	if s.StartSpan == nil {
		s.StartSpan = logging.HTTPSpanStarter(nil, "webserver: ")
	}
	if s.AddEvent == nil {
		s.AddEvent = logging.EventAdder(nil, "webserver: ", nil)
	}
	if s.DetachTrace == nil {
		s.DetachTrace = func(ctx Context) Context { return ctx }
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

	patternPrefix := strings.TrimRight(pattern, "/")
	pathPrefix := strings.TrimLeftFunc(patternPrefix, func(r rune) bool { return r != '/' })

	s.identity = scheme + "://" + s.Authority + pathPrefix + web.Path
	s.pathKnownModules = pathPrefix + web.PathKnownModules

	mux := http.NewServeMux()
	initHandleAPI(mux, patternPrefix+web.Path, s, features)
	initHandleKnownModules(mux, patternPrefix+web.PathKnownModules, s)
	initHandleInstances(mux, patternPrefix+web.PathInstances, s)

	moduleSources := []string{web.KnownModuleSource}

	for _, relURI := range features.ModuleSources {
		sourcePrefix := relURI + "/"
		initHandleModuleSource(mux, patternPrefix+web.PathModule+sourcePrefix, s, sourcePrefix)
		moduleSources = append(moduleSources, strings.TrimLeft(relURI, "/"))
	}

	sort.Strings(moduleSources)
	initHandleStatic(mux, patternPrefix+web.PathModuleSources, s, moduleSources)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if x := recover(); x != nil && x != responded {
				panic(x)
			}
		}()

		_, pattern := mux.Handler(r)

		ctx, end := contextWithSpanEnding(s.StartSpan(r, pattern))
		defer end()

		if !s.anyOrigin {
			if origin := r.Header.Get(web.HeaderOrigin); origin != "" {
				mustBeAllowedOrigin(w, r, s, origin)
			}
		}

		mux.ServeHTTP(w, r.WithContext(ctx))
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

func initHandleStatic(mux *http.ServeMux, pattern string, s *webserver, data any) {
	static := prepareStaticContent(data)

	const methods = "GET, HEAD, OPTIONS"

	mux.HandleFunc("GET "+pattern+"{$}", func(w http.ResponseWriter, r *http.Request) {
		setAccessControl(w, r, s, methods)
		handleGetStatic(w, r, s, &static)
	})

	mux.HandleFunc("OPTIONS "+pattern+"{$}", func(w http.ResponseWriter, r *http.Request) {
		setAccessControl(w, r, s, methods)
		setOptions(w, methods)
	})
}

func initHandleAPI(mux *http.ServeMux, pattern string, s *webserver, features *api.Features) {
	id := s.Server.UUID()

	var token string

	if s.identityKey == nil {
		header := (&web.TokenHeader{Alg: web.SignAlgNone}).MustEncode()
		claims := base64.RawURLEncoding.EncodeToString(must(json.Marshal(web.APIClaims{
			Iss:  s.identity,
			UUID: id,
		})))
		token = fmt.Sprintf("%s.%s.", header, claims)
	} else {
		key := web.PublicKeyEd25519(s.identityKey.Public().(ed25519.PublicKey))
		header := web.TokenHeaderEdDSA(key).MustEncode()
		claims := base64.RawURLEncoding.EncodeToString(must(json.Marshal(web.APIClaims{
			Exp:  time.Now().Add(90 * 24 * time.Hour).Unix(),
			Iss:  s.identity,
			UUID: id,
		})))
		signee := []byte(fmt.Sprintf("%s.%s", header, claims))
		sig := base64.RawURLEncoding.EncodeToString(ed25519.Sign(*s.identityKey, signee))
		token = fmt.Sprintf("%s.%s", signee, sig)
	}

	answers := [3]staticContent{
		prepareStaticContent(web.API{
			UUID: id,
			JWT:  token,
		}),
		prepareStaticContent(web.API{
			UUID:     id,
			JWT:      token,
			Features: &web.Features{Scope: features.Scope},
		}),
		prepareStaticContent(web.API{
			UUID:     id,
			JWT:      token,
			Features: &web.Features{Scope: features.Scope},
		}),
	}

	const methods = "GET, HEAD, OPTIONS"

	mux.HandleFunc("GET "+pattern+"{$}", func(w http.ResponseWriter, r *http.Request) {
		setAccessControl(w, r, s, methods)
		handleGetAPI(w, r, s, &answers)
	})

	mux.HandleFunc("OPTIONS "+pattern+"{$}", func(w http.ResponseWriter, r *http.Request) {
		setAccessControl(w, r, s, methods)
		setOptions(w, methods)
	})
}

func initHandleKnownModules(mux *http.ServeMux, pattern string, s *webserver) {
	{
		const (
			methods = "GET, HEAD, OPTIONS, POST"
			headers = web.HeaderAuthorization
		)

		mux.HandleFunc("GET "+pattern+"{$}", func(w http.ResponseWriter, r *http.Request) {
			setAccessControlAllowHeaders(w, r, s, methods, headers)
		})

		mux.HandleFunc("POST "+pattern+"{$}", func(w http.ResponseWriter, r *http.Request) {
			setAccessControlAllowHeaders(w, r, s, methods, headers)
			handlePostKnownModules(w, r, s)
		})

		// Override the PUT handler with {module...} wildcard.
		mux.HandleFunc("PUT "+pattern+"{$}", func(w http.ResponseWriter, r *http.Request) {
			setAccessControlAllowHeaders(w, r, s, methods, headers)
			w.Header().Set("Allow", methods)
			w.WriteHeader(http.StatusMethodNotAllowed)
		})

		mux.HandleFunc("OPTIONS "+pattern+"{$}", func(w http.ResponseWriter, r *http.Request) {
			setAccessControlAllowHeaders(w, r, s, methods, headers)
			setOptions(w, methods)
		})
	}

	{
		const (
			methods = "GET, HEAD, OPTIONS, POST, PUT"
			headers = web.HeaderAuthorization + ", " + web.HeaderContentType
			exposed = web.HeaderLocation + ", " + web.HeaderInstance + ", " + web.HeaderStatus
		)

		mux.HandleFunc("GET "+pattern+"{module...}", func(w http.ResponseWriter, r *http.Request) {
			setAccessControlAllowExposeHeaders(w, r, s, methods, headers, exposed)
			handleGetKnownModule(w, r, s, r.PathValue("module"))
		})

		mux.HandleFunc("POST "+pattern+"{module...}", func(w http.ResponseWriter, r *http.Request) {
			setAccessControlAllowExposeHeaders(w, r, s, methods, headers, exposed)
			handlePostKnownModule(w, r, s, r.PathValue("module"))
		})

		mux.HandleFunc("PUT "+pattern+"{module...}", func(w http.ResponseWriter, r *http.Request) {
			setAccessControlAllowExposeHeaders(w, r, s, methods, headers, exposed)
			handlePutKnownModule(w, r, s, r.PathValue("module"))
		})

		mux.HandleFunc("OPTIONS "+pattern+"{module...}", func(w http.ResponseWriter, r *http.Request) {
			setAccessControlAllowExposeHeaders(w, r, s, methods, headers, exposed)
			setOptions(w, methods)
		})
	}
}

func initHandleModuleSource(mux *http.ServeMux, pattern string, s *webserver, prefix string) {
	const headers = web.HeaderAuthorization

	{
		// Module directory listing is not supported for sources.  The path
		// clearly exists (it has modules in it), but doesn't support any
		// methods itself.

		const methods = "GET, HEAD, OPTIONS"

		mux.HandleFunc("GET "+pattern+"{$}", func(w http.ResponseWriter, r *http.Request) {
			setAccessControl(w, r, s, methods)
		})

		// Override the POST handler with {module...} wildcard.
		mux.HandleFunc("POST "+pattern+"{$}", func(w http.ResponseWriter, r *http.Request) {
			setAccessControl(w, r, s, methods)
			w.Header().Set("Allow", methods)
			w.WriteHeader(http.StatusMethodNotAllowed)
		})

		mux.HandleFunc("OPTIONS "+pattern+"{$}", func(w http.ResponseWriter, r *http.Request) {
			setAccessControl(w, r, s, methods)
			setOptions(w, methods)
		})
	}

	{
		const (
			methods = "GET, HEAD, OPTIONS, POST"
			exposed = web.HeaderLocation + ", " + web.HeaderInstance + ", " + web.HeaderStatus
		)

		mux.HandleFunc("GET "+pattern+"{module...}", func(w http.ResponseWriter, r *http.Request) {
			setAccessControlAllowExposeHeaders(w, r, s, methods, headers, exposed)
			handleGetModuleSource(w, r, s, prefix+r.PathValue("module"))
		})

		mux.HandleFunc("POST "+pattern+"{module...}", func(w http.ResponseWriter, r *http.Request) {
			setAccessControlAllowExposeHeaders(w, r, s, methods, headers, exposed)
			handlePostModuleSource(w, r, s, prefix+r.PathValue("module"))
		})

		mux.HandleFunc("OPTIONS "+pattern+"{module...}", func(w http.ResponseWriter, r *http.Request) {
			setAccessControlAllowExposeHeaders(w, r, s, methods, headers, exposed)
			setOptions(w, methods)
		})
	}
}

func initHandleInstances(mux *http.ServeMux, pattern string, s *webserver) {
	const exposed = web.HeaderStatus

	{
		const (
			methods = "GET, HEAD, OPTIONS"
			headers = web.HeaderAuthorization
		)

		mux.HandleFunc("GET "+pattern+"{$}", func(w http.ResponseWriter, r *http.Request) {
			setAccessControlAllowHeaders(w, r, s, methods, headers)
		})

		mux.HandleFunc("POST "+pattern+"{$}", func(w http.ResponseWriter, r *http.Request) {
			setAccessControlAllowHeaders(w, r, s, methods, headers)
			handlePostInstances(w, r, s)
		})

		mux.HandleFunc("OPTIONS "+pattern+"{$}", func(w http.ResponseWriter, r *http.Request) {
			setAccessControlAllowHeaders(w, r, s, methods, headers)
			setOptions(w, methods)
		})
	}

	{
		const (
			methods = "GET, HEAD, OPTIONS, POST"
			headers = web.HeaderAuthorization + ", " + web.HeaderContentType
		)

		mux.HandleFunc("GET "+pattern+"{instance...}", func(w http.ResponseWriter, r *http.Request) {
			setAccessControlAllowExposeHeaders(w, r, s, methods, headers, exposed)
			handleGetInstance(w, r, s, r.PathValue("instance"))
		})

		mux.HandleFunc("POST "+pattern+"{instance...}", func(w http.ResponseWriter, r *http.Request) {
			setAccessControlAllowExposeHeaders(w, r, s, methods, headers, exposed)
			handlePostInstance(w, r, s, r.PathValue("instance"))
		})

		mux.HandleFunc("OPTIONS "+pattern+"{instance...}", func(w http.ResponseWriter, r *http.Request) {
			setAccessControlAllowExposeHeaders(w, r, s, methods, headers, exposed)
			setOptions(w, methods)
		})
	}
}

// Method handlers.  Parse query parameters and check content headers.

func handleGetStatic(w http.ResponseWriter, r *http.Request, s *webserver, static *staticContent) {
	mustNotHaveQuery(w, r, s)
	mustAcceptJSON(w, r, s)
	handleStatic(w, r, s, static)
}

func handleGetAPI(w http.ResponseWriter, r *http.Request, s *webserver, answers *[3]staticContent) {
	query := mustParseOptionalQuery(w, r, s)
	features := popOptionalParams(query, web.ParamFeature)
	mustNotHaveParams(w, r, s, query)

	switch {
	case len(features) > 0:
		mustAcceptJSON(w, r, s)
	case len(features) == 0 && !acceptsJSON(r):
		return
	}

	level := 0

	for _, v := range features {
		switch v {
		case web.FeatureScope:
			if level == 0 {
				level = 1
			}

		case web.FeatureAll:
			level = len(*answers) - 1

		default:
			respondUnsupportedFeature(w, r, s)
			return
		}
	}

	handleStatic(w, r, s, &(*answers)[level])
}

func handleGetKnownModule(w http.ResponseWriter, r *http.Request, s *webserver, key string) {
	mustValidateModuleKey(w, r, s, key)

	query := mustParseOptionalQuery(w, r, s)
	pin := popOptionalActionParam(w, r, s, query, web.ActionPin)

	var modTags []string
	if pin {
		modTags = popOptionalParams(query, web.ParamModuleTag)
	}

	if _, found := query[web.ParamAction]; found {
		switch popLastParam(w, r, s, query, web.ParamAction) {
		case web.ActionCall:
			function := mustPopOptionalLastFunctionParam(w, r, s, query)
			instTags := popOptionalParams(query, web.ParamInstanceTag)
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
	mustValidateModuleKey(w, r, s, key)

	mustHaveContentType(w, r, s, web.ContentTypeWebAssembly)
	mustHaveContentLength(w, r, s)
	query := mustParseOptionalQuery(w, r, s)
	pin := popOptionalActionParam(w, r, s, query, web.ActionPin)

	if pin {
		modTags := popOptionalParams(query, web.ParamModuleTag)
		mustNotHaveParams(w, r, s, query)
		handleModuleUpload(w, r, s, key, modTags)
	} else {
		respondUnsupportedAction(w, r, s)
	}
}

func handlePostKnownModule(w http.ResponseWriter, r *http.Request, s *webserver, key string) {
	mustValidateModuleKey(w, r, s, key)

	query := mustParseQuery(w, r, s)

	if len(query[web.ParamAction]) == 0 {
		mustNotHaveParams(w, r, s, query)
		mustNotHaveContentType(w, r, s)
		mustNotHaveContent(w, r, s)
		mustAcceptJSON(w, r, s)
		handleKnownModule(w, r, s, key)
		return
	}

	pin := popOptionalActionParam(w, r, s, query, web.ActionPin)
	suspend := popOptionalActionParam(w, r, s, query, web.ActionSuspend)

	var modTags []string
	if pin {
		modTags = popOptionalParams(query, web.ParamModuleTag)
	}

	if _, found := query[web.ParamAction]; found {
		switch popLastParam(w, r, s, query, web.ParamAction) {
		case web.ActionCall:
			function := mustPopOptionalLastFunctionParam(w, r, s, query)
			instTags := popOptionalParams(query, web.ParamInstanceTag)
			invoke := popOptionalLastLogParam(w, r, s, query)
			mustNotHaveParams(w, r, s, query)

			var op api.Op
			var wasm bool

			if contentType, _ := getContentType(w, r, s); contentType == web.ContentTypeWebAssembly {
				op = api.OpCallUpload
				wasm = true
			} else {
				if pin {
					respondExcessQueryParams(w, r, s)
					return
				}
				op = api.OpCallExtant
			}
			handleCall(w, r, s, op, pin, wasm, "", key, function, modTags, instTags, invoke)

		case web.ActionLaunch:
			function := mustPopOptionalLastFunctionParam(w, r, s, query)
			instance := popOptionalLastParam(w, r, s, query, web.ParamInstance)
			instTags := popOptionalParams(query, web.ParamInstanceTag)
			invoke := popOptionalLastLogParam(w, r, s, query)
			mustNotHaveParams(w, r, s, query)

			if contentType, ok := getContentType(w, r, s); ok {
				if contentType != web.ContentTypeWebAssembly {
					respondUnsupportedMediaType(w, r, s)
					return
				}
				handleLaunchUpload(w, r, s, pin, key, function, instance, modTags, instTags, suspend, invoke)
			} else {
				if pin {
					respondExcessQueryParams(w, r, s)
					return
				}
				mustNotHaveContent(w, r, s)
				handleLaunch(w, r, s, api.OpLaunchExtant, false, "", key, function, instance, nil, instTags, suspend, invoke)
			}

		case web.ActionUnpin:
			if pin {
				respondExcessQueryParams(w, r, s)
				return
			}
			mustNotHaveParams(w, r, s, query)
			mustNotHaveContentType(w, r, s)
			mustNotHaveContent(w, r, s)
			handleModuleUnpin(w, r, s, key)

		default:
			respondUnsupportedAction(w, r, s)
		}
	} else {
		if pin {
			modTags := popOptionalParams(query, web.ParamModuleTag)
			mustNotHaveParams(w, r, s, query)
			mustNotHaveContentType(w, r, s)
			mustNotHaveContent(w, r, s)
			handleModulePin(w, r, s, key, modTags)
		} else {
			respondUnsupportedAction(w, r, s)
		}
	}
}

func handleGetModuleSource(w http.ResponseWriter, r *http.Request, s *webserver, source string) {
	query := mustParseQuery(w, r, s)
	pin := popOptionalActionParam(w, r, s, query, web.ActionPin)

	var modTags []string
	if pin {
		modTags = popOptionalParams(query, web.ParamModuleTag)
	}

	switch popLastParam(w, r, s, query, web.ParamAction) {
	case web.ActionCall:
		function := mustPopOptionalLastFunctionParam(w, r, s, query)
		instTags := popOptionalParams(query, web.ParamInstanceTag)
		invoke := popOptionalLastLogParam(w, r, s, query)
		mustNotHaveParams(w, r, s, query)
		handleCallWebsocket(w, r, s, pin, source, "", function, modTags, instTags, invoke)

	default:
		respondUnsupportedAction(w, r, s)
	}
}

func handlePostModuleSource(w http.ResponseWriter, r *http.Request, s *webserver, source string) {
	query := mustParseQuery(w, r, s)
	pin := popOptionalActionParam(w, r, s, query, web.ActionPin)

	var modTags []string
	if pin {
		modTags = popOptionalParams(query, web.ParamModuleTag)
	}

	if _, found := query[web.ParamAction]; found {
		suspend := popOptionalActionParam(w, r, s, query, web.ActionSuspend)

		switch popLastParam(w, r, s, query, web.ParamAction) {
		case web.ActionCall:
			if suspend {
				respondUnsupportedAction(w, r, s)
			}
			function := mustPopOptionalLastFunctionParam(w, r, s, query)
			instTags := popOptionalParams(query, web.ParamInstanceTag)
			invoke := popOptionalLastLogParam(w, r, s, query)
			mustNotHaveParams(w, r, s, query)
			handleCall(w, r, s, api.OpCallSource, pin, false, source, "", function, modTags, instTags, invoke)

		case web.ActionLaunch:
			function := mustPopOptionalLastFunctionParam(w, r, s, query)
			instance := popOptionalLastParam(w, r, s, query, web.ParamInstance)
			instTags := popOptionalParams(query, web.ParamInstanceTag)
			invoke := popOptionalLastLogParam(w, r, s, query)
			mustNotHaveParams(w, r, s, query)
			mustNotHaveContentType(w, r, s)
			mustNotHaveContent(w, r, s)
			handleLaunch(w, r, s, api.OpLaunchSource, pin, source, "", function, instance, modTags, instTags, suspend, invoke)

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
	mustValidateInstanceIDInPath(w, r, s, instance)

	query := mustParseQuery(w, r, s)

	switch popLastParam(w, r, s, query, web.ParamAction) {
	case web.ActionIO:
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
	mustValidateInstanceIDInPath(w, r, s, instance)

	query := mustParseQuery(w, r, s)
	actions := popOptionalParams(query, web.ParamAction)

	switch len(actions) {
	case 0:
		mustNotHaveParams(w, r, s, query)
		mustAcceptJSON(w, r, s)
		handleInstanceInfo(w, r, s, instance)
		return

	case 1:
		switch actions[0] {
		case web.ActionIO:
			mustNotHaveParams(w, r, s, query)
			handleInstanceConnect(w, r, s, instance)
			return

		case web.ActionResume:
			function := mustPopOptionalLastFunctionParam(w, r, s, query)
			invoke := popOptionalLastLogParam(w, r, s, query)
			mustNotHaveParams(w, r, s, query)
			handleInstanceResume(w, r, s, function, instance, invoke)
			return

		case web.ActionSnapshot:
			modTags := popOptionalParams(query, web.ParamModuleTag)
			mustNotHaveParams(w, r, s, query)
			handleInstanceSnapshot(w, r, s, instance, modTags)
			return

		case web.ActionDelete:
			mustNotHaveParams(w, r, s, query)
			handleInstance(w, r, s, api.OpInstanceDelete, deleteInstance, instance)
			return

		case web.ActionUpdate:
			mustNotHaveParams(w, r, s, query)
			handleInstanceUpdate(w, r, s, instance)
			return

		case web.ActionDebug:
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
		case web.ActionKill:
			kill = true

		case web.ActionSuspend:
			suspend = true

		case web.ActionWait:
			wait = true

		default:
			respondUnsupportedAction(w, r, s)
			return
		}
	}

	switch {
	case kill && !suspend:
		mustNotHaveParams(w, r, s, query)
		handleInstanceWaiter(w, r, s, api.OpInstanceKill, killInstance, instance, wait)

	case suspend && !kill:
		mustNotHaveParams(w, r, s, query)
		handleInstanceWaiter(w, r, s, api.OpInstanceSuspend, suspendInstance, instance, wait)

	case wait && !kill && !suspend:
		mustNotHaveParams(w, r, s, query)
		handleInstanceStatus(w, r, s, api.OpInstanceWait, waitInstance, instance)

	default:
		respondUnsupportedAction(w, r, s)
	}
}

// Action handlers.  Check authorization if needed, and serve the response.

func handleStatic(w http.ResponseWriter, r *http.Request, s *webserver, static *staticContent) {
	w.Header().Set("Cache-Control", cacheControlStatic)
	w.Header().Set(web.HeaderContentLength, static.contentLength)
	w.Header().Set(web.HeaderContentType, contentTypeJSON)
	w.WriteHeader(http.StatusOK)
	w.Write(static.content)
}

func handleModuleList(w http.ResponseWriter, r *http.Request, s *webserver) {
	ctx := r.Context()
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	infos, err := s.Server.Modules(ctx)
	if err != nil {
		respondServerError(ctx, wr, s, "", "", "", "", err)
		return
	}

	content := must(protojson.Marshal(infos))
	w.Header().Set(web.HeaderContentLength, strconv.Itoa(len(content)))
	w.Header().Set(web.HeaderContentType, contentTypeJSON)
	w.WriteHeader(http.StatusOK)
	w.Write(content)
}

func handleKnownModule(w http.ResponseWriter, r *http.Request, s *webserver, key string) {
	ctx := r.Context()
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	info, err := s.Server.ModuleInfo(ctx, key)
	if err != nil {
		respondServerError(ctx, wr, s, "", key, "", "", err)
		return
	}

	content := must(protojson.Marshal(info))
	w.Header().Set(web.HeaderContentLength, strconv.Itoa(len(content)))
	w.Header().Set(web.HeaderContentType, contentTypeJSON)
	w.WriteHeader(http.StatusOK)
	w.Write(content)
}

func handleModuleDownload(w http.ResponseWriter, r *http.Request, s *webserver, key string) {
	ctx := r.Context()
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	content, length, err := s.Server.ModuleContent(ctx, key)
	if err != nil {
		respondServerError(ctx, wr, s, "", key, "", "", err)
		panic(responded)
	}
	defer content.Close()

	w.Header().Set(web.HeaderContentLength, strconv.FormatInt(length, 10))
	w.Header().Set(web.HeaderContentType, web.ContentTypeWebAssembly)
	w.WriteHeader(http.StatusOK)

	if r.Method != "HEAD" {
		io.Copy(w, content)
	}
}

func handleModuleUpload(w http.ResponseWriter, r *http.Request, s *webserver, key string, modTags []string) {
	ctx := r.Context()
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)
	upload := moduleUpload(r.Body, r.ContentLength, key)
	defer upload.Close()

	if _, err := s.Server.UploadModule(ctx, upload, modulePin(true, modTags)); err != nil {
		respondServerError(ctx, wr, s, "", key, "", "", err)
		panic(responded)
	}

	w.WriteHeader(http.StatusCreated)
}

func handleModuleSource(w http.ResponseWriter, r *http.Request, s *webserver, source string, modTags []string) {
	ctx := r.Context()
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	module, err := s.Server.SourceModule(ctx, source, modulePin(true, modTags))
	if err != nil {
		respondServerError(ctx, wr, s, source, "", "", "", err)
		return
	}

	w.Header().Set(web.HeaderLocation, s.pathKnownModules+module)
	w.WriteHeader(http.StatusCreated)
}

func handleModulePin(w http.ResponseWriter, r *http.Request, s *webserver, key string, modTags []string) {
	ctx := r.Context()
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	if err := s.Server.PinModule(ctx, key, modulePin(true, modTags)); err != nil {
		respondServerError(ctx, wr, s, "", key, "", "", err)
		panic(responded)
	}

	w.WriteHeader(http.StatusOK)
}

func handleModuleUnpin(w http.ResponseWriter, r *http.Request, s *webserver, key string) {
	ctx := r.Context()
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	if err := s.Server.UnpinModule(ctx, key); err != nil {
		respondServerError(ctx, wr, s, "", key, "", "", err)
		panic(responded)
	}

	w.WriteHeader(http.StatusOK)
}

func handleCall(w http.ResponseWriter, r *http.Request, s *webserver, op api.Op, pin, wasm bool, source, key, function string, modTags, instTags []string, invoke *api.InvokeOptions) {
	ctx := r.Context()
	trailer := acceptsTrailers(r)
	wr := &requestResponseWriter{w, r}

	launch := &api.LaunchOptions{
		Invoke:    invoke,
		Function:  function,
		Transient: true,
		Tags:      instTags,
	}

	var (
		module string
		inst   api.Instance
		err    error
	)
	switch {
	case wasm:
		ctx = mustParseAuthorizationHeader(ctx, wr, s, pin)
		upload := moduleUpload(r.Body, r.ContentLength, key)
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

	if trailer {
		w.Header().Set("Trailer", web.HeaderStatus)
	}

	if principal.ContextID(ctx) != nil {
		if pin {
			w.Header().Set(web.HeaderLocation, s.pathKnownModules+module)
		}
		w.Header().Set(web.HeaderInstance, inst.ID())
	}

	if pin {
		w.WriteHeader(http.StatusCreated)
	} else {
		w.WriteHeader(http.StatusOK)
	}

	inst.Connect(ctx, r.Body, nopCloser{w})

	status := inst.Wait(ctx)

	if trailer {
		w.Header().Set(web.HeaderStatus, string(must(protojson.Marshal(status))))
	}
}

func handleCallWebsocket(w http.ResponseWriter, r *http.Request, s *webserver, pin bool, source, key, function string, modTags, instTags []string, invoke *api.InvokeOptions) {
	ctx := r.Context()

	conn, err := websocketUpgrader.Upgrade(w, r, nil)
	if err != nil {
		reportProtocolError(ctx, s, err)
		return
	}
	defer conn.Close()

	origCloseHandler := conn.CloseHandler()
	conn.SetCloseHandler(func(code int, text string) error {
		return origCloseHandler(code, text)
	})

	conn.SetReadLimit(maxWebsocketRequestSize)

	var call web.Call
	if err := conn.ReadJSON(&call); err != nil {
		if e := net.Error(nil); errors.As(err, &e) {
			reportNetworkError(ctx, s, err)
		} else {
			reportProtocolError(ctx, s, err)
		}
		return
	}

	conn.SetReadLimit(0)

	var wasm bool

	switch call.ContentType {
	case "":

	case web.ContentTypeWebAssembly:
		if source != "" {
			conn.WriteMessage(websocket.CloseMessage, websocketUnsupportedContent)
			reportProtocolError(ctx, s, errUnsupportedWebsocketContent)
			return
		}
		wasm = true

	default:
		conn.WriteMessage(websocket.CloseMessage, websocketUnsupportedContentType)
		reportProtocolError(ctx, s, errUnsupportedWebsocketContentType)
		return
	}

	launch := &api.LaunchOptions{
		Invoke:    invoke,
		Function:  function,
		Transient: true,
		Tags:      instTags,
	}

	var (
		module string
		inst   api.Instance
	)
	switch {
	case wasm:
		w := websocketResponseWriter{conn}
		ctx = mustParseAuthorization(ctx, w, s, call.Authorization, pin)

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
		upload := moduleUpload(io.NopCloser(frame), call.ContentLength, key)
		defer upload.Close()

		module, inst, err = s.Server.UploadModuleInstance(ctx, upload, modulePin(pin, modTags), launch)
		if err != nil {
			respondServerError(ctx, w, s, "", key, function, "", err)
			return
		}

	case source == "":
		w := websocketResponseWriter{conn}
		ctx = mustParseAuthorization(ctx, w, s, call.Authorization, false)

		module = key
		inst, err = s.Server.NewInstance(ctx, key, launch)
		if err != nil {
			respondServerError(ctx, w, s, "", key, function, "", err)
			return
		}

	default:
		w := websocketResponseWriter{conn}
		ctx = mustParseAuthorization(ctx, w, s, call.Authorization, pin)

		module, inst, err = s.Server.SourceModuleInstance(ctx, source, modulePin(pin, modTags), launch)
		if err != nil {
			// TODO: find out module hash
			respondServerError(ctx, w, s, source, "", function, "", err)
			return
		}
	}
	defer inst.Kill(ctx) // TODO: error

	var reply web.CallConnection
	if principal.ContextID(ctx) != nil {
		if pin {
			reply.Location = s.pathKnownModules + module
		}
		reply.Instance = inst.ID()
	}
	if err := conn.WriteJSON(&reply); err != nil {
		reportNetworkError(ctx, s, err)
		return
	}

	endContextSpan(ctx)
	ctx = s.DetachTrace(ctx)

	handleCallWebsocketIO(ctx, conn, s, inst)
}

func handleCallWebsocketIO(ctx Context, conn *websocket.Conn, s *webserver, inst api.Instance) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	rw := newWebsocketReadWriteCanceler(conn, cancel)
	inst.Connect(ctx, rw, rw)
	status := inst.Wait(ctx)

	data := mustMarshalJSON(web.ConnectionStatus{
		Status: webStatus(status),
	})
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		// TODO: debug?
		return
	}

	conn.WriteMessage(websocket.CloseMessage, websocketNormalClosure)
}

func handleLaunch(w http.ResponseWriter, r *http.Request, s *webserver, op api.Op, pin bool, source, key, function, instance string, modTags, instTags []string, suspend bool, invoke *api.InvokeOptions) {
	ctx := r.Context()
	if instance != "" {
		mustValidateInstanceIDInParam(w, r, s, instance)
	}
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	launch := &api.LaunchOptions{
		Invoke:   invoke,
		Function: function,
		Instance: instance,
		Suspend:  suspend,
		Tags:     instTags,
	}

	var (
		module string
		inst   api.Instance
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

	w.Header().Set(web.HeaderInstance, inst.ID())

	if pin {
		w.Header().Set(web.HeaderLocation, s.pathKnownModules+module)
		w.WriteHeader(http.StatusCreated)
	} else {
		w.WriteHeader(http.StatusOK)
	}
}

func handleLaunchUpload(w http.ResponseWriter, r *http.Request, s *webserver, pin bool, key, function, instance string, modTags, instTags []string, suspend bool, invoke *api.InvokeOptions) {
	ctx := r.Context()
	if instance != "" {
		mustValidateInstanceIDInParam(w, r, s, instance)
	}
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	launch := &api.LaunchOptions{
		Invoke:   invoke,
		Function: function,
		Instance: instance,
		Suspend:  suspend,
		Tags:     instTags,
	}

	upload := moduleUpload(r.Body, r.ContentLength, key)
	defer upload.Close()

	key, inst, err := s.Server.UploadModuleInstance(ctx, upload, modulePin(pin, modTags), launch)
	if err != nil {
		respondServerError(ctx, wr, s, "", key, function, "", err)
		return
	}

	w.Header().Set(web.HeaderInstance, inst.ID())

	if pin {
		w.Header().Set(web.HeaderLocation, s.pathKnownModules+key)
		w.WriteHeader(http.StatusCreated)
	} else {
		w.WriteHeader(http.StatusOK)
	}
}

func handleInstanceList(w http.ResponseWriter, r *http.Request, s *webserver) {
	ctx := r.Context()
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	instances, err := s.Server.Instances(ctx)
	if err != nil {
		respondServerError(ctx, wr, s, "", "", "", "", err)
		return
	}

	content := must(protojson.Marshal(instances))
	w.Header().Set(web.HeaderContentLength, strconv.Itoa(len(content)))
	w.Header().Set(web.HeaderContentType, contentTypeJSON)
	w.WriteHeader(http.StatusOK)
	w.Write(content)
}

func handleInstance(w http.ResponseWriter, r *http.Request, s *webserver, op api.Op, method instanceMethod, instance string) {
	ctx := r.Context()
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	if err := method(ctx, s.Server, instance); err != nil {
		respondServerError(ctx, wr, s, "", "", "", instance, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func handleInstanceInfo(w http.ResponseWriter, r *http.Request, s *webserver, instance string) {
	ctx := r.Context()
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	info, err := s.Server.InstanceInfo(ctx, instance)
	if err != nil {
		respondServerError(ctx, wr, s, "", "", "", instance, err)
		return
	}

	content := must(protojson.Marshal(info))
	w.Header().Set(web.HeaderContentLength, strconv.Itoa(len(content)))
	w.Header().Set(web.HeaderContentType, contentTypeJSON)
	w.WriteHeader(http.StatusOK)
	w.Write(content)
}

func handleInstanceStatus(w http.ResponseWriter, r *http.Request, s *webserver, op api.Op, method instanceStatusMethod, instance string) {
	ctx := r.Context()
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	status, err := method(ctx, s.Server, instance)
	if err != nil {
		respondServerError(ctx, wr, s, "", "", "", instance, err)
		return
	}

	w.Header().Set(web.HeaderStatus, string(must(protojson.Marshal(status))))
	w.WriteHeader(http.StatusOK)
}

func handleInstanceWaiter(w http.ResponseWriter, r *http.Request, s *webserver, op api.Op, method instanceWaiterMethod, instance string, wait bool) {
	ctx := r.Context()
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	inst, err := method(ctx, s.Server, instance)
	if err != nil {
		respondServerError(ctx, wr, s, "", "", "", instance, err)
		return
	}

	if wait {
		status := inst.Wait(ctx)
		w.Header().Set(web.HeaderStatus, string(must(protojson.Marshal(status))))
	}

	w.WriteHeader(http.StatusOK)
}

func handleInstanceResume(w http.ResponseWriter, r *http.Request, s *webserver, function, instance string, invoke *api.InvokeOptions) {
	ctx := r.Context()
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	resume := &api.ResumeOptions{
		Invoke:   invoke,
		Function: function,
	}

	if _, err := s.Server.ResumeInstance(ctx, instance, resume); err != nil {
		respondServerError(ctx, wr, s, "", "", function, instance, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func handleInstanceConnect(w http.ResponseWriter, r *http.Request, s *webserver, instance string) {
	ctx := r.Context()

	iofunc := handleInstanceConnection(ctx, w, r, s, instance)
	if iofunc == nil {
		return
	}

	endContextSpan(ctx)
	ctx = s.DetachTrace(ctx)

	handleInstanceIO(ctx, w, r, s, iofunc)
}

func handleInstanceConnection(ctx Context, w http.ResponseWriter, r *http.Request, s *webserver, instance string) func(Context, io.Reader, io.WriteCloser) *api.Status {
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	_, iofunc, err := s.Server.InstanceConnection(ctx, instance)
	if err != nil {
		respondServerError(ctx, wr, s, "", "", "", instance, err)
		return nil
	}
	if iofunc == nil {
		w.WriteHeader(http.StatusConflict)
		return nil
	}

	if acceptsTrailers(r) {
		w.Header().Set("Trailer", web.HeaderStatus)
	}
	w.WriteHeader(http.StatusOK)

	return iofunc
}

func handleInstanceIO(ctx Context, w http.ResponseWriter, r *http.Request, s *webserver, iofunc func(Context, io.Reader, io.WriteCloser) *api.Status) {
	status := iofunc(ctx, r.Body, nopCloser{w})

	if acceptsTrailers(r) {
		w.Header().Set(web.HeaderStatus, string(must(protojson.Marshal(status))))
	}
}

func handleInstanceConnectWebsocket(w http.ResponseWriter, r *http.Request, s *webserver, instance string) {
	ctx := r.Context()

	conn, err := websocketUpgrader.Upgrade(w, r, nil)
	if err != nil {
		reportProtocolError(ctx, s, err)
		return
	}
	defer conn.Close()

	iofunc := handleInstanceWebsocketConnection(ctx, conn, s, instance)

	endContextSpan(ctx)
	ctx = s.DetachTrace(ctx)

	handleInstanceWebsocketIO(ctx, conn, s, iofunc)
}

func handleInstanceWebsocketConnection(ctx Context, conn *websocket.Conn, s *webserver, instance string) func(Context, io.Reader, io.WriteCloser) *api.Status {
	var req web.IO
	conn.SetReadLimit(maxWebsocketRequestSize)

	if err := conn.ReadJSON(&req); err != nil {
		if e := net.Error(nil); errors.As(err, &e) {
			reportNetworkError(ctx, s, err)
		} else {
			reportProtocolError(ctx, s, err)
		}
		return nil
	}

	conn.SetReadLimit(0)

	w := websocketResponseWriter{conn}
	ctx = mustParseAuthorization(ctx, w, s, req.Authorization, true)

	var ok bool

	_, iofunc, err := s.Server.InstanceConnection(ctx, instance)
	if err != nil {
		respondServerError(ctx, w, s, "", "", "", instance, err)
		conn.WriteMessage(websocket.CloseMessage, websocketNormalClosure)
		return nil
	}
	defer func() {
		if !ok {
			cancelInstanceIO(ctx, iofunc)
		}
	}()

	reply := mustMarshalJSON(web.IOConnection{
		Connected: iofunc != nil,
	})
	if err := conn.WriteMessage(websocket.TextMessage, reply); err != nil {
		reportNetworkError(ctx, s, err)
		return nil
	}

	if iofunc == nil {
		conn.WriteMessage(websocket.CloseMessage, websocketNotConnected)
		return nil
	}

	ok = true
	return iofunc
}

func handleInstanceWebsocketIO(ctx Context, conn *websocket.Conn, s *webserver, iofunc func(Context, io.Reader, io.WriteCloser) *api.Status) {
	rw := newWebsocketReadWriter(conn)
	status := iofunc(ctx, rw, rw)

	data := mustMarshalJSON(web.ConnectionStatus{
		Status: webStatus(status),
	})
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		reportNetworkError(ctx, s, err) // TODO: debug?
		return
	}

	conn.WriteMessage(websocket.CloseMessage, websocketNormalClosure)
}

func cancelInstanceIO(ctx Context, iofunc func(Context, io.Reader, io.WriteCloser) *api.Status) {
	if iofunc == nil {
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	cancel() // Immediately.
	iofunc(ctx, eofReadWriteCloser{}, eofReadWriteCloser{})
}

func handleInstanceSnapshot(w http.ResponseWriter, r *http.Request, s *webserver, instance string, modTags []string) {
	ctx := r.Context()
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	module, err := s.Server.Snapshot(ctx, instance, modulePin(true, modTags))
	if err != nil {
		respondServerError(ctx, wr, s, "", "", "", instance, err)
		return
	}

	w.Header().Set(web.HeaderLocation, s.pathKnownModules+module)
	w.WriteHeader(http.StatusCreated)
}

func handleInstanceUpdate(w http.ResponseWriter, r *http.Request, s *webserver, instance string) {
	ctx := r.Context()
	mustHaveContentType(w, r, s, web.ContentTypeJSON)
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	req := new(api.InstanceUpdate)
	if err := decodeProtoJSON(r.Body, req); err != nil {
		respondContentParseError(ctx, wr, s, err)
		return
	}

	info, err := s.Server.UpdateInstance(ctx, instance, req)
	if err != nil {
		respondServerError(ctx, wr, s, "", "", "", instance, err)
		return
	}

	if !acceptsJSON(r) {
		w.WriteHeader(http.StatusOK)
		return
	}

	content := must(protojson.Marshal(info))
	w.Header().Set(web.HeaderContentLength, strconv.Itoa(len(content)))
	w.Header().Set(web.HeaderContentType, contentTypeJSON)
	w.WriteHeader(http.StatusOK)
	w.Write(content)
}

func handleInstanceDebug(w http.ResponseWriter, r *http.Request, s *webserver, instance string) {
	ctx := r.Context()
	mustHaveContentType(w, r, s, web.ContentTypeJSON)
	wr := &requestResponseWriter{w, r}
	ctx = mustParseAuthorizationHeader(ctx, wr, s, true)

	req := new(api.DebugRequest)
	if err := decodeProtoJSON(r.Body, req); err != nil {
		respondContentParseError(ctx, wr, s, err)
		return
	}

	res, err := s.Server.DebugInstance(ctx, instance, req)
	if err != nil {
		respondServerError(ctx, wr, s, "", "", "", instance, err)
		return
	}

	content := must(protojson.Marshal(res))
	w.Header().Set(web.HeaderContentLength, strconv.Itoa(len(content)))
	w.Header().Set(web.HeaderContentType, contentTypeJSON)
	w.WriteHeader(http.StatusOK)
	w.Write(content)
}

func moduleUpload(s io.ReadCloser, length int64, hash string) *api.ModuleUpload {
	return &api.ModuleUpload{
		Stream: s,
		Length: length,
		Hash:   hash,
	}
}

func modulePin(pin bool, tags []string) *api.ModuleOptions {
	return &api.ModuleOptions{
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

type eofReadWriteCloser struct{}

func (eofReadWriteCloser) Read([]byte) (int, error)  { return 0, io.EOF }
func (eofReadWriteCloser) Write([]byte) (int, error) { return 0, io.EOF }
func (eofReadWriteCloser) Close() error              { return nil }
