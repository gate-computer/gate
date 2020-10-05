// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gate_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"gate.computer/gate/runtime/abi"
	"gate.computer/gate/server"
	"gate.computer/gate/server/database"
	"gate.computer/gate/server/database/sql"
	"gate.computer/gate/server/web"
	"gate.computer/gate/service"
	"gate.computer/gate/service/origin"
	"gate.computer/gate/service/plugin"
	"gate.computer/gate/snapshot/wasm"
	"gate.computer/gate/webapi"
	"gate.computer/gate/webapi/authorization"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"github.com/tsavola/wag"
	"github.com/tsavola/wag/binding"
	"github.com/tsavola/wag/compile"
	"github.com/tsavola/wag/section"
	"golang.org/x/crypto/ed25519"
)

type principalKey struct {
	privateKey  ed25519.PrivateKey
	tokenHeader []byte
}

var weakRand = rand.New(rand.NewSource(0))

func newPrincipalKey() principalKey {
	pub, pri, err := ed25519.GenerateKey(weakRand)
	if err != nil {
		panic(err)
	}

	return principalKey{pri, webapi.TokenHeaderEdDSA(webapi.PublicKeyEd25519(pub)).MustEncode()}
}

func (pri principalKey) authorization(claims *webapi.Claims) (s string) {
	s, err := authorization.BearerEd25519(pri.privateKey, pri.tokenHeader, claims)
	if err != nil {
		panic(err)
	}

	return
}

type helloSource struct{}

func (helloSource) OpenURI(ctx context.Context, uri string, maxSize int,
) (contentLength int64, content io.ReadCloser, err error) {
	switch uri {
	case "/test/hello":
		contentLength = int64(len(wasmHello))
		content = ioutil.NopCloser(bytes.NewReader(wasmHello))
		return

	default:
		panic(uri)
	}
}

var nonceChecker database.NonceChecker

func init() {
	db, err := sql.OpenNonceChecker(context.Background(), sql.Config{
		Driver: "sqlite3",
		DSN:    "file::memory:?cache=shared",
	})
	if err != nil {
		panic(err)
	}

	nonceChecker = db
}

func newServices() func(context.Context) server.InstanceServices {
	registry := new(service.Registry)

	plugins, err := plugin.OpenAll("lib/gate/plugin")
	if err != nil {
		panic(err)
	}

	err = plugins.InitServices(context.Background(), registry)
	if err != nil {
		panic(err)
	}

	return func(ctx context.Context) server.InstanceServices {
		connector := origin.New(origin.Config{})
		r := registry.Clone()
		r.MustRegister(connector)
		return server.NewInstanceServices(connector, r)
	}
}

func newServer() (*server.Server, error) {
	return server.New(context.Background(), server.Config{
		ProcessFactory: newExecutor(),
		AccessPolicy:   server.NewPublicAccess(newServices()),
	})
}

func newHandler(t *testing.T) http.Handler {
	t.Helper()

	s, err := newServer()
	if err != nil {
		t.Fatal(err)
	}

	config := web.Config{
		Server:        s,
		Authority:     "example.invalid",
		NonceStorage:  nonceChecker,
		ModuleSources: map[string]server.Source{"/test": helloSource{}},
	}

	h := web.NewHandler("/", config)
	// h = handlers.LoggingHandler(os.Stdout, h)

	return h
}

func newRequest(method, path string, content []byte) (req *http.Request) {
	var body io.ReadCloser
	if content != nil {
		body = ioutil.NopCloser(bytes.NewReader(content))
	}
	req = httptest.NewRequest(method, path, body)
	req.ContentLength = int64(len(content))
	return
}

func newSignedRequest(pri principalKey, method, path string, content []byte) (req *http.Request) {
	req = newRequest(method, path, content)
	req.Header.Set(webapi.HeaderAuthorization, pri.authorization(&webapi.Claims{
		Exp:   time.Now().Add(time.Minute).Unix(),
		Aud:   []string{"no", "https://example.invalid/gate-0/"},
		Nonce: strconv.Itoa(rand.Int()),
	}))
	return
}

func doRequest(t *testing.T, handler http.Handler, req *http.Request,
) (resp *http.Response, content []byte) {
	t.Helper()

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	resp = w.Result()
	defer resp.Body.Close()

	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("response content error: %v", err)
	}
	return
}

func checkResponse(t *testing.T, handler http.Handler, req *http.Request, expectStatusCode int,
) (resp *http.Response, content []byte) {
	t.Helper()

	resp, content = doRequest(t, handler, req)
	if resp.StatusCode != expectStatusCode {
		if len(content) > 0 {
			t.Logf("response content: %q", content)
		}
		t.Fatalf("response status: %s", resp.Status)
	}
	return
}

func checkStatusHeader(t *testing.T, statusHeader string, expect webapi.Status) {
	t.Helper()

	var status webapi.Status

	if err := json.Unmarshal([]byte(statusHeader), &status); err != nil {
		t.Error(err)
	}

	if !reflect.DeepEqual(status, expect) {
		t.Errorf("%#v", status)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	all := []string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut}

	for path, methods := range map[string][]string{
		webapi.Path:                                 all,
		webapi.PathModule:                           all,
		webapi.PathModules:                          []string{http.MethodPost, http.MethodPut},
		webapi.Path + "instance":                    all,
		webapi.PathInstances:                        []string{http.MethodPost, http.MethodPut},
		webapi.PathInstances + "id":                 []string{http.MethodHead, http.MethodPut},
		webapi.PathModules + webapi.ModuleRefSource: all,
		webapi.PathModuleRefs:                       []string{http.MethodPost, http.MethodPut},
	} {
		for _, method := range methods {
			req := httptest.NewRequest(method, path, nil)
			checkResponse(t, newHandler(t), req, http.StatusMethodNotAllowed)
		}
	}
}

func TestModuleSourceList(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, webapi.PathModules, nil)
	resp, content := checkResponse(t, newHandler(t), req, http.StatusOK)

	if x := resp.Header.Get(webapi.HeaderContentType); x != "application/json; charset=utf-8" {
		t.Error(x)
	}

	var sources interface{}

	if err := json.Unmarshal(content, &sources); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(sources, []interface{}{
		webapi.ModuleRefSource,
		"test",
	}) {
		t.Errorf("%#v", sources)
	}
}

func checkModuleList(t *testing.T, handler http.Handler, pri principalKey, expect interface{}) {
	t.Helper()

	req := newSignedRequest(pri, http.MethodGet, webapi.PathModuleRefs, nil)
	resp, content := checkResponse(t, handler, req, http.StatusOK)

	if x := resp.Header.Get(webapi.HeaderContentType); x != "application/json; charset=utf-8" {
		t.Error(x)
	}

	var refs interface{}

	if err := json.Unmarshal(content, &refs); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(refs, expect) {
		t.Errorf("%#v", refs)
	}
}

func TestModuleRef(t *testing.T) {
	handler := newHandler(t)
	pri := newPrincipalKey()

	t.Run("ListEmpty", func(t *testing.T) {
		checkModuleList(t, handler, pri, map[string]interface{}{
			"modules": []interface{}{},
		})
	})

	t.Run("Put", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPut, webapi.PathModuleRefs+hashHello, wasmHello)
		req.Header.Set(webapi.HeaderContentType, webapi.ContentTypeWebAssembly)
		resp, content := checkResponse(t, handler, req, http.StatusNoContent)

		if s, found := resp.Header[webapi.HeaderContentType]; found {
			t.Errorf("%q", s)
		}

		if len(content) != 0 {
			t.Error(content)
		}

		checkModuleList(t, handler, pri, map[string]interface{}{
			"modules": []interface{}{},
		})
	})

	t.Run("PutRef", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPut, webapi.PathModuleRefs+hashHello+"?action=ref", wasmHello)
		req.Header.Set(webapi.HeaderContentType, webapi.ContentTypeWebAssembly)
		resp, content := checkResponse(t, handler, req, http.StatusCreated)

		if s, found := resp.Header[webapi.HeaderContentType]; found {
			t.Errorf("%q", s)
		}

		if len(content) != 0 {
			t.Error(content)
		}
	})

	t.Run("ListOne", func(t *testing.T) {
		checkModuleList(t, handler, pri, map[string]interface{}{
			"modules": []interface{}{
				map[string]interface{}{
					"id": hashHello,
				},
			},
		})
	})

	t.Run("PutWrongHash", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPut, webapi.PathModuleRefs+sha384([]byte("asdf")), wasmHello)
		req.Header.Set(webapi.HeaderContentType, webapi.ContentTypeWebAssembly)
		checkResponse(t, handler, req, http.StatusBadRequest)
	})

	t.Run("ListStillOne", func(t *testing.T) {
		checkModuleList(t, handler, pri, map[string]interface{}{
			"modules": []interface{}{
				map[string]interface{}{
					"id": hashHello,
				},
			},
		})
	})

	t.Run("Get", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodGet, webapi.PathModuleRefs+hashHello, nil)
		resp, content := checkResponse(t, handler, req, http.StatusOK)

		if x := resp.Header.Get(webapi.HeaderContentType); x != webapi.ContentTypeWebAssembly {
			t.Error(x)
		}

		if !bytes.Equal(content, wasmHello) {
			t.Error(content)
		}
	})

	t.Run("GetNotFound", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodGet, webapi.PathModuleRefs+"3R_g4HTkvIb0sx8-ppwrrJRu3T6rT5mpA3SvAmifGMmGzYB7xIAMbS9qmax5WigT", nil)
		resp, content := checkResponse(t, handler, req, http.StatusNotFound)

		if x := resp.Header.Get(webapi.HeaderContentType); x != "text/plain; charset=utf-8" {
			t.Error(x)
		}

		if string(content) != "module not found\n" {
			t.Errorf("%q", content)
		}
	})

	for _, spec := range [][2]string{
		{"", ""},
		{"greet", "hello, world\n"},
		{"twice", "hello, world\nhello, world\n"},
	} {
		fn := spec[0]
		expect := spec[1]

		t.Run("Call"+strings.Title(fn), func(t *testing.T) {
			req := newSignedRequest(pri, http.MethodPost, webapi.PathModuleRefs+hashHello+"?action=call&function="+fn, nil)
			resp, content := checkResponse(t, handler, req, http.StatusOK)

			if s, found := resp.Header[webapi.HeaderContentType]; found {
				t.Errorf("%q", s)
			}

			if _, err := uuid.Parse(resp.Header.Get(webapi.HeaderInstance)); err != nil {
				t.Error(err)
			}

			if string(content) != expect {
				t.Errorf("%q", content)
			}

			checkStatusHeader(t, resp.Trailer.Get(webapi.HeaderStatus), webapi.Status{
				State: webapi.StateTerminated,
			})

			if len(resp.Trailer) != 1 {
				t.Errorf("trailer: %v", resp.Trailer)
			}
		})

		t.Run("Launch"+strings.Title(fn), func(t *testing.T) {
			req := newSignedRequest(pri, http.MethodPost, webapi.PathModuleRefs+hashHello+"?action=launch&function="+fn, nil)
			resp, content := checkResponse(t, handler, req, http.StatusNoContent)

			if s, found := resp.Header[webapi.HeaderContentType]; found {
				t.Errorf("%q", s)
			}

			if _, err := uuid.Parse(resp.Header.Get(webapi.HeaderInstance)); err != nil {
				t.Error(err)
			}

			if len(content) != 0 {
				t.Errorf("%q", content)
			}
		})
	}

	t.Run("Unref", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPost, webapi.PathModuleRefs+hashHello+"?action=unref", nil)
		resp, content := checkResponse(t, handler, req, http.StatusNoContent)

		if s, found := resp.Header[webapi.HeaderContentType]; found {
			t.Errorf("%q", s)
		}

		if len(content) != 0 {
			t.Error(content)
		}

		checkModuleList(t, handler, pri, map[string]interface{}{
			"modules": []interface{}{},
		})
	})

	t.Run("UnrefNotFound", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPost, webapi.PathModuleRefs+hashHello+"?action=unref", nil)
		checkResponse(t, handler, req, http.StatusNotFound)
	})

	t.Run("PutCall", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPut, webapi.PathModuleRefs+hashHello+"?action=call", wasmHello)
		req.Header.Set(webapi.HeaderContentType, webapi.ContentTypeWebAssembly)
		resp, content := checkResponse(t, handler, req, http.StatusOK)

		if s, found := resp.Header[webapi.HeaderContentType]; found {
			t.Errorf("%q", s)
		}

		if _, err := uuid.Parse(resp.Header.Get(webapi.HeaderInstance)); err != nil {
			t.Error(err)
		}

		if len(content) != 0 {
			t.Errorf("%q", content)
		}

		checkStatusHeader(t, resp.Trailer.Get(webapi.HeaderStatus), webapi.Status{
			State: webapi.StateTerminated,
		})

		if len(resp.Trailer) != 1 {
			t.Errorf("trailer: %v", resp.Trailer)
		}

		checkModuleList(t, handler, pri, map[string]interface{}{
			"modules": []interface{}{},
		})

		req = newSignedRequest(pri, http.MethodPost, webapi.PathModuleRefs+hashHello+"?action=unref", nil)
		checkResponse(t, handler, req, http.StatusNotFound)
	})

	t.Run("PutRefCall", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPut, webapi.PathModuleRefs+hashHello+"?action=ref&action=call", wasmHello)
		req.Header.Set(webapi.HeaderContentType, webapi.ContentTypeWebAssembly)
		resp, content := checkResponse(t, handler, req, http.StatusCreated)

		if s, found := resp.Header[webapi.HeaderContentType]; found {
			t.Errorf("%q", s)
		}

		if _, err := uuid.Parse(resp.Header.Get(webapi.HeaderInstance)); err != nil {
			t.Error(err)
		}

		if len(content) != 0 {
			t.Errorf("%q", content)
		}

		checkStatusHeader(t, resp.Trailer.Get(webapi.HeaderStatus), webapi.Status{
			State: webapi.StateTerminated,
		})

		if len(resp.Trailer) != 1 {
			t.Errorf("trailer: %v", resp.Trailer)
		}

		checkModuleList(t, handler, pri, map[string]interface{}{
			"modules": []interface{}{
				map[string]interface{}{
					"id": hashHello,
				},
			},
		})

		req = newSignedRequest(pri, http.MethodPost, webapi.PathModuleRefs+hashHello+"?action=unref", nil)
		checkResponse(t, handler, req, http.StatusNoContent)
	})

	t.Run("PutLaunch", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPut, webapi.PathModuleRefs+hashHello+"?action=launch", wasmHello)
		req.Header.Set(webapi.HeaderContentType, webapi.ContentTypeWebAssembly)
		resp, content := checkResponse(t, handler, req, http.StatusNoContent)

		if s, found := resp.Header[webapi.HeaderContentType]; found {
			t.Errorf("%q", s)
		}

		if _, err := uuid.Parse(resp.Header.Get(webapi.HeaderInstance)); err != nil {
			t.Error(err)
		}

		if len(content) != 0 {
			t.Errorf("%q", content)
		}

		checkModuleList(t, handler, pri, map[string]interface{}{
			"modules": []interface{}{},
		})

		req = newSignedRequest(pri, http.MethodPost, webapi.PathModuleRefs+hashHello+"?action=unref", nil)
		checkResponse(t, handler, req, http.StatusNotFound)
	})

	t.Run("PutRefLaunch", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPut, webapi.PathModuleRefs+hashHello+"?action=ref&action=launch", wasmHello)
		req.Header.Set(webapi.HeaderContentType, webapi.ContentTypeWebAssembly)
		resp, content := checkResponse(t, handler, req, http.StatusCreated)

		if s, found := resp.Header[webapi.HeaderContentType]; found {
			t.Errorf("%q", s)
		}

		if _, err := uuid.Parse(resp.Header.Get(webapi.HeaderInstance)); err != nil {
			t.Error(err)
		}

		if len(content) != 0 {
			t.Errorf("%q", content)
		}

		checkModuleList(t, handler, pri, map[string]interface{}{
			"modules": []interface{}{
				map[string]interface{}{
					"id": hashHello,
				},
			},
		})

		req = newSignedRequest(pri, http.MethodPost, webapi.PathModuleRefs+hashHello+"?action=unref", nil)
		checkResponse(t, handler, req, http.StatusNoContent)
	})

	t.Run("LaunchUpload", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPut, webapi.PathModuleRefs+hashHello+"?action=launch", wasmHello)
		req.Header.Set(webapi.HeaderContentType, webapi.ContentTypeWebAssembly)
		resp, content := checkResponse(t, handler, req, http.StatusNoContent)

		if s, found := resp.Header[webapi.HeaderLocation]; found {
			t.Errorf("%q", s)
		}

		if s, found := resp.Header[webapi.HeaderContentType]; found {
			t.Errorf("%q", s)
		}

		if _, err := uuid.Parse(resp.Header.Get(webapi.HeaderInstance)); err != nil {
			t.Error(err)
		}

		if len(content) != 0 {
			t.Error(content)
		}

		req = newSignedRequest(pri, http.MethodPost, webapi.PathModuleRefs+hashHello+"?action=unref", nil)
		checkResponse(t, handler, req, http.StatusNotFound)
	})

	t.Run("RefLaunchUpload", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPut, webapi.PathModuleRefs+hashHello+"?action=launch&action=ref", wasmHello)
		req.Header.Set(webapi.HeaderContentType, webapi.ContentTypeWebAssembly)
		resp, content := checkResponse(t, handler, req, http.StatusCreated)

		if x := resp.Header.Get(webapi.HeaderLocation); x != webapi.PathModuleRefs+hashHello {
			t.Error(x)
		}

		if s, found := resp.Header[webapi.HeaderContentType]; found {
			t.Errorf("%q", s)
		}

		if _, err := uuid.Parse(resp.Header.Get(webapi.HeaderInstance)); err != nil {
			t.Error(err)
		}

		if len(content) != 0 {
			t.Error(content)
		}

		req = newSignedRequest(pri, http.MethodPost, webapi.PathModuleRefs+hashHello+"?action=unref", nil)
		checkResponse(t, handler, req, http.StatusNoContent)
	})

	t.Run("ActionNotImplemented", func(t *testing.T) {
		req := newRequest(http.MethodPut, webapi.PathModuleRefs+hashHello+"?action=bad", wasmHello)
		req.Header.Set(webapi.HeaderContentType, webapi.ContentTypeWebAssembly)
		checkResponse(t, handler, req, http.StatusNotImplemented)

		req = newSignedRequest(pri, http.MethodPut, webapi.PathModuleRefs+hashHello+"?action=bad", wasmHello)
		req.Header.Set(webapi.HeaderContentType, webapi.ContentTypeWebAssembly)
		checkResponse(t, handler, req, http.StatusNotImplemented)

		req = newSignedRequest(pri, http.MethodPost, webapi.PathModuleRefs+hashHello+"?action=bad", nil)
		checkResponse(t, handler, req, http.StatusNotImplemented)
	})
}

func TestModuleSource(t *testing.T) {
	handler := newHandler(t)
	pri := newPrincipalKey()

	t.Run("Post", func(t *testing.T) {
		req := newRequest(http.MethodPost, webapi.PathModule+"/test/hello", nil)
		resp, content := checkResponse(t, handler, req, http.StatusNoContent)

		if s, found := resp.Header[webapi.HeaderContentType]; found {
			t.Errorf("%q", s)
		}
		if s, found := resp.Header[webapi.HeaderLocation]; found {
			t.Errorf("%q", s)
		}
		if s, found := resp.Header[webapi.HeaderInstance]; found {
			t.Errorf("%q", s)
		}

		if len(content) > 0 {
			t.Errorf("%q", content)
		}

		checkModuleList(t, handler, pri, map[string]interface{}{
			"modules": []interface{}{},
		})

		req = newSignedRequest(pri, http.MethodGet, webapi.PathModuleRefs+hashHello, nil)
		checkResponse(t, handler, req, http.StatusNotFound)
	})

	t.Run("PostRef", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPost, webapi.PathModule+"/test/hello?action=ref", nil)
		resp, _ := checkResponse(t, handler, req, http.StatusCreated)

		if s, found := resp.Header[webapi.HeaderContentType]; found {
			t.Errorf("%q", s)
		}
		if s := resp.Header.Get(webapi.HeaderLocation); s != webapi.PathModuleRefs+hashHello {
			t.Errorf("%q", s)
		}
		if s, found := resp.Header[webapi.HeaderInstance]; found {
			t.Errorf("%q", s)
		}

		checkModuleList(t, handler, pri, map[string]interface{}{
			"modules": []interface{}{
				map[string]interface{}{
					"id": hashHello,
				},
			},
		})

		req = newSignedRequest(pri, http.MethodGet, webapi.PathModuleRefs+hashHello, nil)
		checkResponse(t, handler, req, http.StatusOK)
	})

	for _, spec := range [][2]string{
		{"", ""},
		{"greet", "hello, world\n"},
		{"twice", "hello, world\nhello, world\n"},
	} {
		fn := spec[0]
		expect := spec[1]

		t.Run("AnonCall"+strings.Title(fn), func(t *testing.T) {
			req := newRequest(http.MethodPost, webapi.PathModule+"/test/hello?action=call&function="+fn, nil)
			resp, content := checkResponse(t, handler, req, http.StatusOK)

			if s, found := resp.Header[webapi.HeaderContentType]; found {
				t.Errorf("%q", s)
			}
			if s, found := resp.Header[webapi.HeaderLocation]; found {
				t.Errorf("%q", s)
			}
			if s, found := resp.Header[webapi.HeaderInstance]; found {
				t.Errorf("%q", s)
			}

			if string(content) != expect {
				t.Errorf("%q", content)
			}

			checkStatusHeader(t, resp.Trailer.Get(webapi.HeaderStatus), webapi.Status{
				State: webapi.StateTerminated,
			})

			if len(resp.Trailer) != 1 {
				t.Errorf("trailer: %v", resp.Trailer)
			}
		})

		t.Run("AnonRefCallUnauthorized"+strings.Title(fn), func(t *testing.T) {
			req := newRequest(http.MethodPost, webapi.PathModule+"/test/hello?action=ref&action=call&function="+fn, nil)
			checkResponse(t, handler, req, http.StatusUnauthorized)
		})

		t.Run("Call"+strings.Title(fn), func(t *testing.T) {
			req := newSignedRequest(pri, http.MethodPost, webapi.PathModuleRefs+hashHello+"?action=unref", nil)
			doRequest(t, handler, req)

			req = newSignedRequest(pri, http.MethodPost, webapi.PathModule+"/test/hello?action=call&function="+fn, nil)
			resp, content := checkResponse(t, handler, req, http.StatusOK)

			if s, found := resp.Header[webapi.HeaderLocation]; found {
				t.Errorf("%q", s)
			}

			if s, found := resp.Header[webapi.HeaderContentType]; found {
				t.Errorf("%q", s)
			}

			if _, err := uuid.Parse(resp.Header.Get(webapi.HeaderInstance)); err != nil {
				t.Error(err)
			}

			if string(content) != expect {
				t.Errorf("%q", content)
			}

			checkStatusHeader(t, resp.Trailer.Get(webapi.HeaderStatus), webapi.Status{
				State: webapi.StateTerminated,
			})

			if len(resp.Trailer) != 1 {
				t.Errorf("trailer: %v", resp.Trailer)
			}

			req = newSignedRequest(pri, http.MethodPost, webapi.PathModuleRefs+hashHello+"?action=unref", nil)
			checkResponse(t, handler, req, http.StatusNotFound)
		})

		t.Run("RefCall"+strings.Title(fn), func(t *testing.T) {
			req := newSignedRequest(pri, http.MethodPost, webapi.PathModule+"/test/hello?action=ref&action=call&function="+fn, nil)
			resp, content := checkResponse(t, handler, req, http.StatusCreated)

			if x := resp.Header.Get(webapi.HeaderLocation); x != webapi.PathModuleRefs+hashHello {
				t.Error(x)
			}

			if s, found := resp.Header[webapi.HeaderContentType]; found {
				t.Errorf("%q", s)
			}

			if _, err := uuid.Parse(resp.Header.Get(webapi.HeaderInstance)); err != nil {
				t.Error(err)
			}

			if string(content) != expect {
				t.Errorf("%q", content)
			}

			checkStatusHeader(t, resp.Trailer.Get(webapi.HeaderStatus), webapi.Status{
				State: webapi.StateTerminated,
			})

			if len(resp.Trailer) != 1 {
				t.Errorf("trailer: %v", resp.Trailer)
			}

			req = newSignedRequest(pri, http.MethodPost, webapi.PathModuleRefs+hashHello+"?action=unref", nil)
			checkResponse(t, handler, req, http.StatusNoContent)
		})

		t.Run("Launch"+strings.Title(fn), func(t *testing.T) {
			req := newSignedRequest(pri, http.MethodPost, webapi.PathModule+"/test/hello?action=launch&function="+fn, nil)
			resp, content := checkResponse(t, handler, req, http.StatusNoContent)

			if s, found := resp.Header[webapi.HeaderLocation]; found {
				t.Errorf("%q", s)
			}

			if s, found := resp.Header[webapi.HeaderContentType]; found {
				t.Errorf("%q", s)
			}

			if _, err := uuid.Parse(resp.Header.Get(webapi.HeaderInstance)); err != nil {
				t.Error(err)
			}

			if len(content) != 0 {
				t.Error(content)
			}

			req = newSignedRequest(pri, http.MethodPost, webapi.PathModuleRefs+hashHello+"?action=unref", nil)
			checkResponse(t, handler, req, http.StatusNotFound)
		})

		t.Run("RefLaunch"+strings.Title(fn), func(t *testing.T) {
			req := newSignedRequest(pri, http.MethodPost, webapi.PathModule+"/test/hello?action=ref&action=launch&function="+fn, nil)
			resp, content := checkResponse(t, handler, req, http.StatusCreated)

			if x := resp.Header.Get(webapi.HeaderLocation); x != webapi.PathModuleRefs+hashHello {
				t.Error(x)
			}

			if s, found := resp.Header[webapi.HeaderContentType]; found {
				t.Errorf("%q", s)
			}

			if _, err := uuid.Parse(resp.Header.Get(webapi.HeaderInstance)); err != nil {
				t.Error(err)
			}

			if len(content) != 0 {
				t.Error(content)
			}

			req = newSignedRequest(pri, http.MethodPost, webapi.PathModuleRefs+hashHello+"?action=unref", nil)
			checkResponse(t, handler, req, http.StatusNoContent)
		})
	}

	t.Run("CallPluginTest", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPost, webapi.PathModule+"/test/hello?action=call&function=test_plugin&action=ref", nil)
		checkResponse(t, handler, req, http.StatusCreated)
	})

	t.Run("Ref", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodHead, webapi.PathModuleRefs+hashHello, nil)
		resp, content := checkResponse(t, handler, req, http.StatusOK)

		if x := resp.Header.Get(webapi.HeaderContentType); x != webapi.ContentTypeWebAssembly {
			t.Error(x)
		}

		if len(content) != 0 {
			t.Error(content)
		}
	})

	t.Run("ActionNotImplemented", func(t *testing.T) {
		req := newRequest(http.MethodPost, webapi.PathModule+"/test/hello?action=bad", nil)
		checkResponse(t, handler, req, http.StatusNotImplemented)

		req = newSignedRequest(pri, http.MethodPost, webapi.PathModule+"/test/hello?action=bad", nil)
		checkResponse(t, handler, req, http.StatusNotImplemented)
	})
}

func checkInstanceList(t *testing.T, handler http.Handler, pri principalKey, expect interface{}) {
	t.Helper()

	req := newSignedRequest(pri, http.MethodGet, webapi.PathInstances, nil)
	resp, content := checkResponse(t, handler, req, http.StatusOK)

	if x := resp.Header.Get(webapi.HeaderContentType); x != "application/json; charset=utf-8" {
		t.Error(x)
	}

	var ids interface{}

	if err := json.Unmarshal(content, &ids); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(ids, expect) {
		t.Errorf("%#v", ids)
	}
}

func checkInstanceStatus(t *testing.T, handler http.Handler, pri principalKey, instID string, expect webapi.Status) {
	t.Helper()

	req := newSignedRequest(pri, http.MethodPost, webapi.PathInstances+instID+"?action=status", nil)
	resp, content := checkResponse(t, handler, req, http.StatusNoContent)

	if s, found := resp.Header[webapi.HeaderContentType]; found {
		t.Errorf("%q", s)
	}

	if len(content) != 0 {
		t.Error(content)
	}

	checkStatusHeader(t, resp.Header.Get(webapi.HeaderStatus), expect)
}

func TestInstance(t *testing.T) {
	handler := newHandler(t)
	pri := newPrincipalKey()

	t.Run("ListEmpty", func(t *testing.T) {
		checkInstanceList(t, handler, pri, map[string]interface{}{
			"instances": []interface{}{},
		})
	})

	var instID string

	{
		req := newSignedRequest(pri, http.MethodPut, webapi.PathModuleRefs+hashHello+"?action=ref&action=launch&function=greet", wasmHello)
		req.Header.Set(webapi.HeaderContentType, webapi.ContentTypeWebAssembly)
		resp, _ := checkResponse(t, handler, req, http.StatusCreated)

		instID = resp.Header.Get(webapi.HeaderInstance)
	}

	t.Run("StatusRunning", func(t *testing.T) {
		checkInstanceStatus(t, handler, pri, instID, webapi.Status{
			State: webapi.StateRunning,
		})
	})

	t.Run("ListOne", func(t *testing.T) {
		checkInstanceList(t, handler, pri, map[string]interface{}{
			"instances": []interface{}{
				map[string]interface{}{
					"instance": instID,
					"status": map[string]interface{}{
						"state": webapi.StateRunning,
					},
				},
			},
		})
	})

	t.Run("DeleteFail", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPost, webapi.PathInstances+instID+"?action=delete", nil)
		checkResponse(t, handler, req, http.StatusBadRequest)
	})

	t.Run("IO", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPost, webapi.PathInstances+instID+"?action=io", nil)
		resp, content := checkResponse(t, handler, req, http.StatusOK)

		if s, found := resp.Header[webapi.HeaderContentType]; found {
			t.Errorf("%q", s)
		}

		if string(content) != "hello, world\n" {
			t.Errorf("%q", content)
		}

		if !(resp.Trailer.Get(webapi.HeaderStatus) == `{"state":"RUNNING"}` || resp.Trailer.Get(webapi.HeaderStatus) == `{"state":"HALTED"}`) || len(resp.Trailer) != 1 {
			t.Errorf("trailer: %v", resp.Trailer)
		}
	})

	t.Run("Wait", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPost, webapi.PathInstances+instID+"?action=wait", nil)
		resp, _ := checkResponse(t, handler, req, http.StatusNoContent)

		checkStatusHeader(t, resp.Header.Get(webapi.HeaderStatus), webapi.Status{
			State: webapi.StateHalted,
		})
	})

	t.Run("StatusHalted", func(t *testing.T) {
		checkInstanceStatus(t, handler, pri, instID, webapi.Status{
			State: webapi.StateHalted,
		})
	})

	t.Run("ActionNotImplemented", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPost, webapi.PathInstances+instID+"?action=bad", nil)
		checkResponse(t, handler, req, http.StatusNotImplemented)
	})

	t.Run("Delete", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPost, webapi.PathInstances+instID+"?action=delete", nil)
		checkResponse(t, handler, req, http.StatusNoContent)
	})

	t.Run("ListEmptyAgain", func(t *testing.T) {
		checkInstanceList(t, handler, pri, map[string]interface{}{
			"instances": []interface{}{},
		})
	})
}

func TestInstanceMultiIO(t *testing.T) {
	handler := newHandler(t)
	pri := newPrincipalKey()

	var instID string

	{
		req := newSignedRequest(pri, http.MethodPut, webapi.PathModuleRefs+hashHello+"?action=ref&action=launch&function=multi", wasmHello)
		req.Header.Set(webapi.HeaderContentType, webapi.ContentTypeWebAssembly)
		resp, _ := checkResponse(t, handler, req, http.StatusCreated)

		instID = resp.Header.Get(webapi.HeaderInstance)
	}

	done := make(chan struct{}, 10)

	for i := 0; i < cap(done); i++ {
		go func() {
			defer func() { done <- struct{}{} }()

			req := newSignedRequest(pri, http.MethodPost, webapi.PathInstances+instID+"?action=io", nil)
			resp, content := checkResponse(t, handler, req, http.StatusOK)

			if s, found := resp.Header[webapi.HeaderContentType]; found {
				t.Errorf("%q", s)
			}

			if string(content) != "hello, world\n" {
				t.Errorf("%q", content)
			}

			checkStatusHeader(t, resp.Trailer.Get(webapi.HeaderStatus), webapi.Status{
				State: webapi.StateRunning,
			})

			if len(resp.Trailer) != 1 {
				t.Errorf("trailer: %v", resp.Trailer)
			}
		}()
	}

	for i := 0; i < cap(done); i++ {
		<-done
	}
}

func TestInstanceKill(t *testing.T) {
	handler := newHandler(t)
	pri := newPrincipalKey()

	var instID string

	{
		req := newSignedRequest(pri, http.MethodPut, webapi.PathModuleRefs+hashHello+"?action=launch&function=multi", wasmHello)
		req.Header.Set(webapi.HeaderContentType, webapi.ContentTypeWebAssembly)
		resp, _ := checkResponse(t, handler, req, http.StatusNoContent)

		instID = resp.Header.Get(webapi.HeaderInstance)
	}

	t.Run("KillWait", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPost, webapi.PathInstances+instID+"?action=kill&action=wait", nil)
		resp, _ := checkResponse(t, handler, req, http.StatusNoContent)

		checkStatusHeader(t, resp.Header.Get(webapi.HeaderStatus), webapi.Status{
			State: webapi.StateKilled,
		})
	})

	t.Run("Status", func(t *testing.T) {
		checkInstanceStatus(t, handler, pri, instID, webapi.Status{
			State: webapi.StateKilled,
		})
	})
}

func TestInstanceSuspend(t *testing.T) {
	handler := newHandler(t)
	pri := newPrincipalKey()

	var instID string

	{
		req := newSignedRequest(pri, http.MethodPut, webapi.PathModuleRefs+hashSuspend+"?action=launch&function=loop&debug=true", wasmSuspend)
		req.Header.Set(webapi.HeaderContentType, webapi.ContentTypeWebAssembly)
		resp, _ := checkResponse(t, handler, req, http.StatusNoContent)

		instID = resp.Header.Get(webapi.HeaderInstance)
	}

	if testing.Verbose() {
		time.Sleep(time.Second / 3)
	}

	t.Run("Suspend", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPost, webapi.PathInstances+instID+"?action=suspend", nil)
		resp, _ := checkResponse(t, handler, req, http.StatusNoContent)

		if x := resp.Header[webapi.HeaderStatus]; x != nil {
			t.Error(x)
		}
	})

	t.Run("Wait", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPost, webapi.PathInstances+instID+"?action=wait", nil)
		resp, _ := checkResponse(t, handler, req, http.StatusNoContent)

		checkStatusHeader(t, resp.Header.Get(webapi.HeaderStatus), webapi.Status{
			State: webapi.StateSuspended,
		})
	})

	var snapshot []byte

	t.Run("Snapshot", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPost, webapi.PathInstances+instID+"?action=snapshot", nil)
		resp, _ := checkResponse(t, handler, req, http.StatusCreated)

		location := resp.Header.Get(webapi.HeaderLocation)
		if location == "" {
			t.Fatal("no module location")
		}

		req = newSignedRequest(pri, http.MethodGet, location, nil)
		resp, snapshot = checkResponse(t, handler, req, http.StatusOK)

		if false {
			f, err := os.Create("/tmp/snapshot.wasm")
			if err != nil {
				t.Error(err)
			} else {
				defer f.Close()
				if _, err := f.Write(snapshot); err != nil {
					t.Error(err)
				}
			}
		}

		config := &wag.Config{ImportResolver: new(abi.ImportResolver)}
		if _, err := wag.Compile(config, bytes.NewReader(snapshot), abi.Library()); err != nil {
			t.Error(err)
		}
	})

	t.Run("ResumeFunction", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPost, webapi.PathInstances+instID+"?action=resume&function=loop", nil)
		checkResponse(t, handler, req, http.StatusBadRequest)
	})

	t.Run("Resume", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPost, webapi.PathInstances+instID+"?action=resume&debug=true", nil)
		checkResponse(t, handler, req, http.StatusNoContent)

		if testing.Verbose() {
			time.Sleep(time.Second / 3)
		}

		checkInstanceStatus(t, handler, pri, instID, webapi.Status{
			State: webapi.StateRunning,
		})

		if testing.Verbose() {
			time.Sleep(time.Second / 3)
		}

		req = newSignedRequest(pri, http.MethodPost, webapi.PathInstances+instID+"?action=suspend", nil)
		checkResponse(t, handler, req, http.StatusNoContent)

		req = newSignedRequest(pri, http.MethodPost, webapi.PathInstances+instID+"?action=wait", nil)
		resp, _ := checkResponse(t, handler, req, http.StatusNoContent)

		checkStatusHeader(t, resp.Header.Get(webapi.HeaderStatus), webapi.Status{
			State: webapi.StateSuspended,
		})
	})

	handler2 := newHandler(t)

	t.Run("Restore", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPut, webapi.PathModuleRefs+sha384(snapshot)+"?action=launch&debug=true", snapshot)
		req.Header.Set(webapi.HeaderContentType, webapi.ContentTypeWebAssembly)
		resp, _ := checkResponse(t, handler2, req, http.StatusNoContent)
		restoredID := resp.Header.Get(webapi.HeaderInstance)

		if testing.Verbose() {
			time.Sleep(time.Second / 3)
		}

		req = newSignedRequest(pri, http.MethodPost, webapi.PathInstances+restoredID+"?action=suspend", nil)
		checkResponse(t, handler2, req, http.StatusNoContent)

		req = newSignedRequest(pri, http.MethodPost, webapi.PathInstances+restoredID+"?action=wait", nil)
		resp, _ = checkResponse(t, handler2, req, http.StatusNoContent)

		checkStatusHeader(t, resp.Header.Get(webapi.HeaderStatus), webapi.Status{
			State: webapi.StateSuspended,
		})
	})
}

func TestInstanceTerminated(t *testing.T) {
	handler := newHandler(t)
	pri := newPrincipalKey()

	var instID string

	{
		req := newSignedRequest(pri, http.MethodPut, webapi.PathModuleRefs+hashHello+"?action=launch&function=fail", wasmHello)
		req.Header.Set(webapi.HeaderContentType, webapi.ContentTypeWebAssembly)
		resp, _ := checkResponse(t, handler, req, http.StatusNoContent)

		instID = resp.Header.Get(webapi.HeaderInstance)
	}

	t.Run("Wait", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPost, webapi.PathInstances+instID+"?action=wait", nil)
		resp, _ := checkResponse(t, handler, req, http.StatusNoContent)

		checkStatusHeader(t, resp.Header.Get(webapi.HeaderStatus), webapi.Status{
			State:  webapi.StateTerminated,
			Result: 1,
		})
	})

	t.Run("Snapshot", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPost, webapi.PathInstances+instID+"?action=snapshot", nil)
		resp, _ := checkResponse(t, handler, req, http.StatusCreated)

		location := resp.Header.Get(webapi.HeaderLocation)
		if location == "" {
			t.Fatal("no module location")
		}

		req = newSignedRequest(pri, http.MethodGet, location, nil)
		resp, snapshot := checkResponse(t, handler, req, http.StatusOK)

		if false {
			f, err := os.Create("/tmp/snapshot.wasm")
			if err != nil {
				t.Error(err)
			} else {
				defer f.Close()
				if _, err := f.Write(snapshot); err != nil {
					t.Error(err)
				}
			}
		}

		var final bool

		loaders := map[string]section.CustomContentLoader{
			wasm.SectionSnapshot: func(_ string, r section.Reader, _ uint32) (err error) {
				snap, _, err := wasm.ReadSnapshotSection(r)
				if err != nil {
					return
				}

				final = snap.Final()
				return
			},
		}

		c := compile.Config{CustomSectionLoader: section.CustomLoader(loaders)}
		r := bytes.NewReader(snapshot)

		m, err := compile.LoadInitialSections(&compile.ModuleConfig{Config: c}, r)
		if err != nil {
			t.Fatal(err)
		}

		binding.BindImports(&m, new(abi.ImportResolver))

		if err := compile.LoadCodeSection(&compile.CodeConfig{Config: c}, r, m, abi.Library()); err != nil {
			t.Fatal(err)
		}

		if err := compile.LoadDataSection(&compile.DataConfig{Config: c}, r, m); err != nil {
			t.Fatal(err)
		}

		if err := compile.LoadCustomSections(&c, r); err != nil {
			t.Error(err)
		}

		if !final {
			t.Error("snapshot section did not have final flag set")
		}
	})
}

// TODO: WebSocket tests
