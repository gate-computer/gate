// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gate_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"gate.computer/gate/runtime/abi"
	"gate.computer/gate/scope/program/system"
	"gate.computer/gate/server"
	"gate.computer/gate/server/webserver"
	"gate.computer/gate/service"
	"gate.computer/gate/service/origin"
	"gate.computer/gate/snapshot/wasm"
	"gate.computer/gate/source"
	"gate.computer/gate/web"
	_ "gate.computer/internal/test/service-ext"
	"gate.computer/wag"
	"gate.computer/wag/binding"
	"gate.computer/wag/compile"
	"gate.computer/wag/section"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	. "import.name/testing/mustr"
	. "import.name/type/context"
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

	return principalKey{pri, web.TokenHeaderEdDSA(web.PublicKeyEd25519(pub)).MustEncode()}
}

func (pri principalKey) authorization(claims *web.AuthorizationClaims) (s string) {
	s, err := web.AuthorizationBearerEd25519(pri.privateKey, pri.tokenHeader, claims)
	if err != nil {
		panic(err)
	}

	return
}

type helloSource struct{}

func (helloSource) CanonicalURI(uri string) (string, error) {
	return uri, nil
}

func (helloSource) OpenURI(ctx Context, uri string, maxSize int) (io.ReadCloser, int64, error) {
	switch uri {
	case "/test/hello":
		return io.NopCloser(bytes.NewReader(wasmHello)), int64(len(wasmHello)), nil

	default:
		panic(uri)
	}
}

type debugLog struct{}

func openDebugLog(string) io.WriteCloser     { return debugLog{} }
func (debugLog) Write(b []byte) (int, error) { return os.Stdout.Write(b) }
func (debugLog) Close() error                { return nil }

func newServices() func(Context) server.InstanceServices {
	registry := new(service.Registry)

	if err := service.Init(context.Background(), registry); err != nil {
		panic(err)
	}

	return func(ctx Context) server.InstanceServices {
		connector := origin.New(nil)
		r := registry.Clone()
		r.MustRegister(connector)
		return server.NewInstanceServices(connector, r)
	}
}

func newServer() (*server.Server, error) {
	return server.New(context.Background(), &server.Config{
		UUID:           uuid.NewString(),
		ProcessFactory: newExecutor(),
		Inventory:      newTestInventory(),
		AccessPolicy:   server.NewPublicAccess(newServices()),
		ModuleSources:  map[string]source.Source{"/test": helloSource{}},
		SourceCache:    newTestSourceCache(),
		OpenDebugLog:   openDebugLog,
	})
}

func newHandler(t *testing.T) http.Handler {
	t.Helper()

	config := &webserver.Config{
		Server:       Must(t, R(newServer())),
		Authority:    "example.invalid",
		Origins:      []string{"null"},
		NonceChecker: newTestNonceChecker(),
	}

	h := webserver.NewHandler("/", config)
	// h = handlers.LoggingHandler(os.Stdout, h)
	return h
}

func newRequest(method, path string, content []byte) (req *http.Request) {
	var body io.ReadCloser
	if content != nil {
		body = io.NopCloser(bytes.NewReader(content))
	}
	req = httptest.NewRequest(method, path, body)
	req.ContentLength = int64(len(content))
	req.Header.Set(web.HeaderOrigin, "null")
	return
}

func newSignedRequest(pri principalKey, method, path string, content []byte) (req *http.Request) {
	req = newRequest(method, path, content)
	req.Header.Set(web.HeaderAuthorization, pri.authorization(&web.AuthorizationClaims{
		Exp:   time.Now().Add(time.Minute).Unix(),
		Aud:   []string{"no", "https://example.invalid/gate-0/"},
		Nonce: strconv.Itoa(rand.Int()),
	}))
	return
}

func doRequest(t *testing.T, handler http.Handler, req *http.Request) (*http.Response, []byte) {
	t.Helper()

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	resp := w.Result()
	defer resp.Body.Close()

	content := Must(t, R(io.ReadAll(resp.Body)))
	return resp, content
}

func checkResponse(t *testing.T, handler http.Handler, req *http.Request, expectStatusCode int) (*http.Response, []byte) {
	t.Helper()

	resp, content := doRequest(t, handler, req)
	if resp.StatusCode != expectStatusCode {
		if len(content) > 0 {
			t.Logf("response content: %q", content)
		}
		t.Fatalf("response status: %s", resp.Status)
	}

	return resp, content
}

func checkStatusHeader(t *testing.T, statusHeader string, expect web.Status) {
	t.Helper()

	var status web.Status
	assert.NoError(t, json.Unmarshal([]byte(statusHeader), &status))
	assert.Equal(t, status, expect)
}

func TestOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, web.Path, nil)
	checkResponse(t, newHandler(t), req, http.StatusOK)

	req = httptest.NewRequest(http.MethodGet, web.Path, nil)
	req.Header.Set(web.HeaderOrigin, "https://example.net")
	checkResponse(t, newHandler(t), req, http.StatusForbidden)

	req = httptest.NewRequest(http.MethodGet, web.Path, nil)
	req.Header.Set(web.HeaderOrigin, "null")
	checkResponse(t, newHandler(t), req, http.StatusOK)
}

func TestRedirect(t *testing.T) {
	for path, location := range map[string]string{
		web.PathModule: web.PathModuleSources,
		web.PathModuleSources + web.KnownModuleSource: web.PathKnownModules,
		web.PathModuleSources + "test":                web.PathModuleSources + "test/",
		web.Path + "instance":                         web.PathInstances,
	} {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			req := httptest.NewRequest(method, path, nil)
			req.Header.Set(web.HeaderOrigin, "null")

			resp, _ := checkResponse(t, newHandler(t), req, http.StatusMovedPermanently)
			assert.Equal(t, resp.Header.Get("Location"), location)
		}
	}
}

func TestMethodNotAllowed(t *testing.T) {
	for path, methods := range map[string][]string{
		web.Path:                 {http.MethodPost, http.MethodPut},
		web.PathModuleSources:    {http.MethodPost, http.MethodPut},
		web.PathInstances:        {http.MethodPut},
		web.PathInstances + "id": {http.MethodPut},
		web.PathKnownModules:     {http.MethodPut},
	} {
		for _, method := range methods {
			req := httptest.NewRequest(method, path, nil)
			req.Header.Set(web.HeaderOrigin, "null")
			resp, _ := checkResponse(t, newHandler(t), req, http.StatusMethodNotAllowed)
			assert.NotEmpty(t, resp.Header.Get("Allow"))
		}
	}
}

func TestFeatures(t *testing.T) {
	expectScope := []string{system.Scope}

	for query, expect := range map[string]*web.Features{
		"":                         nil,
		"?feature=scope":           {Scope: expectScope},
		"?feature=*":               {Scope: expectScope},
		"?feature=*&feature=scope": {Scope: expectScope},
	} {
		req := httptest.NewRequest(http.MethodGet, web.Path+query, nil)
		_, content := checkResponse(t, newHandler(t), req, http.StatusOK)

		var api *web.API
		require.NoError(t, json.Unmarshal(content, &api))
		assert.Equal(t, api.Features, expect)
	}

	for query, expect := range map[string]int{
		"?foo=bar":     http.StatusBadRequest,
		"?feature=baz": http.StatusNotImplemented,
	} {
		req := httptest.NewRequest(http.MethodGet, web.Path+query, nil)
		checkResponse(t, newHandler(t), req, expect)
	}
}

func TestModuleSourceList(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, web.PathModuleSources, nil)
	req.Header.Set(web.HeaderOrigin, "null")
	resp, content := checkResponse(t, newHandler(t), req, http.StatusOK)

	assert.Equal(t, resp.Header.Get(web.HeaderContentType), "application/json; charset=utf-8")

	var sources any
	require.NoError(t, json.Unmarshal(content, &sources))
	assert.Equal(t, sources, []any{
		web.KnownModuleSource,
		"test",
	})
}

func checkModuleList(t *testing.T, handler http.Handler, pri principalKey, expect any) {
	t.Helper()

	req := newSignedRequest(pri, http.MethodPost, web.PathKnownModules, nil)
	resp, content := checkResponse(t, handler, req, http.StatusOK)

	assert.Equal(t, resp.Header.Get(web.HeaderContentType), "application/json; charset=utf-8")

	var refs any
	require.NoError(t, json.Unmarshal(content, &refs))
	assert.Equal(t, refs, expect)
}

func TestKnownModule(t *testing.T) {
	handler := newHandler(t)
	pri := newPrincipalKey()

	t.Run("ListEmpty", func(t *testing.T) {
		checkModuleList(t, handler, pri, map[string]any{})
	})

	t.Run("Put", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPut, web.PathKnownModules+hashHello, wasmHello)
		req.Header.Set(web.HeaderContentType, web.ContentTypeWebAssembly)
		checkResponse(t, handler, req, http.StatusNotImplemented)
	})

	t.Run("PutPin", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPut, web.PathKnownModules+hashHello+"?action=pin", wasmHello)
		req.Header.Set(web.HeaderContentType, web.ContentTypeWebAssembly)
		resp, content := checkResponse(t, handler, req, http.StatusCreated)

		if s, found := resp.Header[web.HeaderContentType]; found {
			t.Errorf("%q", s)
		}

		assert.Empty(t, content)
	})

	t.Run("ListOne", func(t *testing.T) {
		checkModuleList(t, handler, pri, map[string]any{
			"modules": []any{
				map[string]any{
					"module": hashHello,
				},
			},
		})
	})

	t.Run("PutWrongHash", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPut, web.PathKnownModules+sha256hex([]byte("asdf"))+"?action=pin", wasmHello)
		req.Header.Set(web.HeaderContentType, web.ContentTypeWebAssembly)
		checkResponse(t, handler, req, http.StatusBadRequest)
	})

	t.Run("ListStillOne", func(t *testing.T) {
		checkModuleList(t, handler, pri, map[string]any{
			"modules": []any{
				map[string]any{
					"module": hashHello,
				},
			},
		})
	})

	t.Run("Get", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodGet, web.PathKnownModules+hashHello, nil)
		resp, content := checkResponse(t, handler, req, http.StatusOK)

		assert.Equal(t, resp.Header.Get(web.HeaderContentType), web.ContentTypeWebAssembly)
		assert.Equal(t, content, wasmHello)
	})

	t.Run("GetNotFound", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodGet, web.PathKnownModules+"C5879EADB14FA246BD9DCB2EA3D91CDC6461D33DD87F88774BE2BDE7F9AB5149", nil)
		resp, content := checkResponse(t, handler, req, http.StatusNotFound)

		assert.Equal(t, resp.Header.Get(web.HeaderContentType), "text/plain; charset=utf-8")
		assert.Equal(t, string(content), "module hash must use lower-case hex encoding\n")
	})

	for _, spec := range [][2]string{
		{"", ""},
		{"greet", "hello, world\n"},
		{"twice", "hello, world\nhello, world\n"},
	} {
		fn := spec[0]
		expect := spec[1]

		t.Run("Call_"+fn, func(t *testing.T) {
			req := newSignedRequest(pri, http.MethodPost, web.PathKnownModules+hashHello+"?action=call&function="+fn, nil)
			req.Header.Set(web.HeaderTE, web.TETrailers)
			resp, content := checkResponse(t, handler, req, http.StatusOK)

			if s, found := resp.Header[web.HeaderContentType]; found {
				t.Errorf("%q", s)
			}

			Must(t, R(uuid.Parse(resp.Header.Get(web.HeaderInstance))))

			assert.Equal(t, string(content), expect)

			checkStatusHeader(t, resp.Trailer.Get(web.HeaderStatus), web.Status{
				State: web.StateTerminated,
			})

			assert.Equal(t, len(resp.Trailer), 1)
		})

		t.Run("Launch_"+fn, func(t *testing.T) {
			req := newSignedRequest(pri, http.MethodPost, web.PathKnownModules+hashHello+"?action=launch&function="+fn, nil)
			resp, content := checkResponse(t, handler, req, http.StatusOK)

			if s, found := resp.Header[web.HeaderContentType]; found {
				t.Errorf("%q", s)
			}

			Must(t, R(uuid.Parse(resp.Header.Get(web.HeaderInstance))))

			assert.Empty(t, content)
		})
	}

	t.Run("Unpin", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPost, web.PathKnownModules+hashHello+"?action=unpin", nil)
		resp, content := checkResponse(t, handler, req, http.StatusOK)

		if s, found := resp.Header[web.HeaderContentType]; found {
			t.Errorf("%q", s)
		}

		assert.Empty(t, content)

		checkModuleList(t, handler, pri, map[string]any{})
	})

	t.Run("UnpinNotFound", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPost, web.PathKnownModules+hashHello+"?action=unpin", nil)
		checkResponse(t, handler, req, http.StatusNotFound)
	})

	t.Run("PostCall", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPost, web.PathKnownModules+hashHello+"?action=call", wasmHello)
		req.Header.Set(web.HeaderContentType, web.ContentTypeWebAssembly)
		req.Header.Set(web.HeaderTE, web.TETrailers)
		resp, content := checkResponse(t, handler, req, http.StatusOK)

		if s, found := resp.Header[web.HeaderContentType]; found {
			t.Errorf("%q", s)
		}

		Must(t, R(uuid.Parse(resp.Header.Get(web.HeaderInstance))))

		assert.Empty(t, content)

		checkStatusHeader(t, resp.Trailer.Get(web.HeaderStatus), web.Status{
			State: web.StateTerminated,
		})

		assert.Equal(t, len(resp.Trailer), 1)

		checkModuleList(t, handler, pri, map[string]any{})

		req = newSignedRequest(pri, http.MethodPost, web.PathKnownModules+hashHello+"?action=unpin", nil)
		checkResponse(t, handler, req, http.StatusNotFound)
	})

	t.Run("PostPinCall", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPost, web.PathKnownModules+hashHello+"?action=pin&action=call", wasmHello)
		req.Header.Set(web.HeaderContentType, web.ContentTypeWebAssembly)
		req.Header.Set(web.HeaderTE, web.TETrailers)
		resp, content := checkResponse(t, handler, req, http.StatusCreated)

		if s, found := resp.Header[web.HeaderContentType]; found {
			t.Errorf("%q", s)
		}

		Must(t, R(uuid.Parse(resp.Header.Get(web.HeaderInstance))))

		assert.Empty(t, content)

		checkStatusHeader(t, resp.Trailer.Get(web.HeaderStatus), web.Status{
			State: web.StateTerminated,
		})

		assert.Equal(t, len(resp.Trailer), 1)

		checkModuleList(t, handler, pri, map[string]any{
			"modules": []any{
				map[string]any{
					"module": hashHello,
				},
			},
		})

		req = newSignedRequest(pri, http.MethodPost, web.PathKnownModules+hashHello+"?action=unpin", nil)
		checkResponse(t, handler, req, http.StatusOK)
	})

	t.Run("PostLaunch", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPost, web.PathKnownModules+hashHello+"?action=launch", wasmHello)
		req.Header.Set(web.HeaderContentType, web.ContentTypeWebAssembly)
		resp, content := checkResponse(t, handler, req, http.StatusOK)

		if s, found := resp.Header[web.HeaderContentType]; found {
			t.Errorf("%q", s)
		}

		Must(t, R(uuid.Parse(resp.Header.Get(web.HeaderInstance))))

		assert.Empty(t, content)

		checkModuleList(t, handler, pri, map[string]any{})

		req = newSignedRequest(pri, http.MethodPost, web.PathKnownModules+hashHello+"?action=unpin", nil)
		checkResponse(t, handler, req, http.StatusNotFound)
	})

	t.Run("PostPinLaunch", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPost, web.PathKnownModules+hashHello+"?action=pin&action=launch", wasmHello)
		req.Header.Set(web.HeaderContentType, web.ContentTypeWebAssembly)
		resp, content := checkResponse(t, handler, req, http.StatusCreated)

		if s, found := resp.Header[web.HeaderContentType]; found {
			t.Errorf("%q", s)
		}

		Must(t, R(uuid.Parse(resp.Header.Get(web.HeaderInstance))))

		assert.Empty(t, content)

		checkModuleList(t, handler, pri, map[string]any{
			"modules": []any{
				map[string]any{
					"module": hashHello,
				},
			},
		})

		req = newSignedRequest(pri, http.MethodPost, web.PathKnownModules+hashHello+"?action=unpin", nil)
		checkResponse(t, handler, req, http.StatusOK)
	})

	t.Run("LaunchUpload", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPost, web.PathKnownModules+hashHello+"?action=launch", wasmHello)
		req.Header.Set(web.HeaderContentType, web.ContentTypeWebAssembly)
		resp, content := checkResponse(t, handler, req, http.StatusOK)

		if s, found := resp.Header[web.HeaderLocation]; found {
			t.Errorf("%q", s)
		}

		if s, found := resp.Header[web.HeaderContentType]; found {
			t.Errorf("%q", s)
		}

		Must(t, R(uuid.Parse(resp.Header.Get(web.HeaderInstance))))

		assert.Empty(t, content)

		req = newSignedRequest(pri, http.MethodPost, web.PathKnownModules+hashHello+"?action=unpin", nil)
		checkResponse(t, handler, req, http.StatusNotFound)
	})

	t.Run("PinLaunchUpload", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPost, web.PathKnownModules+hashHello+"?action=launch&action=pin", wasmHello)
		req.Header.Set(web.HeaderContentType, web.ContentTypeWebAssembly)
		resp, content := checkResponse(t, handler, req, http.StatusCreated)

		assert.Equal(t, resp.Header.Get(web.HeaderLocation), web.PathKnownModules+hashHello)

		if s, found := resp.Header[web.HeaderContentType]; found {
			t.Errorf("%q", s)
		}

		Must(t, R(uuid.Parse(resp.Header.Get(web.HeaderInstance))))

		assert.Empty(t, content)

		req = newSignedRequest(pri, http.MethodPost, web.PathKnownModules+hashHello+"?action=unpin", nil)
		checkResponse(t, handler, req, http.StatusOK)
	})

	t.Run("ActionNotImplemented", func(t *testing.T) {
		req := newRequest(http.MethodPut, web.PathKnownModules+hashHello+"?action=bad", wasmHello)
		req.Header.Set(web.HeaderContentType, web.ContentTypeWebAssembly)
		checkResponse(t, handler, req, http.StatusNotImplemented)

		req = newRequest(http.MethodPost, web.PathKnownModules+hashHello+"?action=bad", wasmHello)
		req.Header.Set(web.HeaderContentType, web.ContentTypeWebAssembly)
		checkResponse(t, handler, req, http.StatusNotImplemented)

		req = newSignedRequest(pri, http.MethodPut, web.PathKnownModules+hashHello+"?action=bad", wasmHello)
		req.Header.Set(web.HeaderContentType, web.ContentTypeWebAssembly)
		checkResponse(t, handler, req, http.StatusNotImplemented)

		req = newSignedRequest(pri, http.MethodPost, web.PathKnownModules+hashHello+"?action=bad", nil)
		checkResponse(t, handler, req, http.StatusNotImplemented)
	})
}

func TestModuleSource(t *testing.T) {
	handler := newHandler(t)
	pri := newPrincipalKey()

	t.Run("Post", func(t *testing.T) {
		req := newRequest(http.MethodPost, web.PathModule+"/test/hello", nil)
		checkResponse(t, handler, req, http.StatusNotImplemented)
	})

	t.Run("PostPin", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPost, web.PathModule+"/test/hello?action=pin", nil)
		resp, _ := checkResponse(t, handler, req, http.StatusCreated)

		if s, found := resp.Header[web.HeaderContentType]; found {
			t.Errorf("%q", s)
		}
		assert.Equal(t, resp.Header.Get(web.HeaderLocation), web.PathKnownModules+hashHello)
		if s, found := resp.Header[web.HeaderInstance]; found {
			t.Errorf("%q", s)
		}

		checkModuleList(t, handler, pri, map[string]any{
			"modules": []any{
				map[string]any{
					"module": hashHello,
				},
			},
		})

		req = newSignedRequest(pri, http.MethodGet, web.PathKnownModules+hashHello, nil)
		checkResponse(t, handler, req, http.StatusOK)
	})

	for _, spec := range [][2]string{
		{"", ""},
		{"greet", "hello, world\n"},
		{"twice", "hello, world\nhello, world\n"},
	} {
		fn := spec[0]
		expect := spec[1]

		t.Run("AnonCall_"+fn, func(t *testing.T) {
			req := newRequest(http.MethodPost, web.PathModule+"/test/hello?action=call&function="+fn, nil)
			req.Header.Set(web.HeaderTE, web.TETrailers)
			resp, content := checkResponse(t, handler, req, http.StatusOK)

			if s, found := resp.Header[web.HeaderContentType]; found {
				t.Errorf("%q", s)
			}
			if s, found := resp.Header[web.HeaderLocation]; found {
				t.Errorf("%q", s)
			}
			if s, found := resp.Header[web.HeaderInstance]; found {
				t.Errorf("%q", s)
			}

			assert.Equal(t, string(content), expect)

			checkStatusHeader(t, resp.Trailer.Get(web.HeaderStatus), web.Status{
				State: web.StateTerminated,
			})

			assert.Equal(t, len(resp.Trailer), 1)
		})

		t.Run("AnonPinCallUnauthorized_"+fn, func(t *testing.T) {
			req := newRequest(http.MethodPost, web.PathModule+"/test/hello?action=pin&action=call&function="+fn, nil)
			checkResponse(t, handler, req, http.StatusUnauthorized)
		})

		t.Run("Call_"+fn, func(t *testing.T) {
			req := newSignedRequest(pri, http.MethodPost, web.PathKnownModules+hashHello+"?action=unpin", nil)
			doRequest(t, handler, req)

			req = newSignedRequest(pri, http.MethodPost, web.PathModule+"/test/hello?action=call&function="+fn, nil)
			req.Header.Set(web.HeaderTE, web.TETrailers)
			resp, content := checkResponse(t, handler, req, http.StatusOK)

			if s, found := resp.Header[web.HeaderLocation]; found {
				t.Errorf("%q", s)
			}

			if s, found := resp.Header[web.HeaderContentType]; found {
				t.Errorf("%q", s)
			}

			Must(t, R(uuid.Parse(resp.Header.Get(web.HeaderInstance))))

			assert.Equal(t, string(content), expect)

			checkStatusHeader(t, resp.Trailer.Get(web.HeaderStatus), web.Status{
				State: web.StateTerminated,
			})

			assert.Equal(t, len(resp.Trailer), 1)

			req = newSignedRequest(pri, http.MethodPost, web.PathKnownModules+hashHello+"?action=unpin", nil)
			checkResponse(t, handler, req, http.StatusNotFound)
		})

		t.Run("PinCall_"+fn, func(t *testing.T) {
			req := newSignedRequest(pri, http.MethodPost, web.PathModule+"/test/hello?action=pin&action=call&function="+fn, nil)
			req.Header.Set(web.HeaderTE, web.TETrailers)
			resp, content := checkResponse(t, handler, req, http.StatusCreated)

			assert.Equal(t, resp.Header.Get(web.HeaderLocation), web.PathKnownModules+hashHello)

			if s, found := resp.Header[web.HeaderContentType]; found {
				t.Errorf("%q", s)
			}

			Must(t, R(uuid.Parse(resp.Header.Get(web.HeaderInstance))))

			assert.Equal(t, string(content), expect)

			checkStatusHeader(t, resp.Trailer.Get(web.HeaderStatus), web.Status{
				State: web.StateTerminated,
			})

			assert.Equal(t, len(resp.Trailer), 1)

			req = newSignedRequest(pri, http.MethodPost, web.PathKnownModules+hashHello+"?action=unpin", nil)
			checkResponse(t, handler, req, http.StatusOK)
		})

		t.Run("Launch_"+fn, func(t *testing.T) {
			req := newSignedRequest(pri, http.MethodPost, web.PathModule+"/test/hello?action=launch&function="+fn, nil)
			resp, content := checkResponse(t, handler, req, http.StatusOK)

			if s, found := resp.Header[web.HeaderLocation]; found {
				t.Errorf("%q", s)
			}

			if s, found := resp.Header[web.HeaderContentType]; found {
				t.Errorf("%q", s)
			}

			Must(t, R(uuid.Parse(resp.Header.Get(web.HeaderInstance))))

			assert.Empty(t, content)

			req = newSignedRequest(pri, http.MethodPost, web.PathKnownModules+hashHello+"?action=unpin", nil)
			checkResponse(t, handler, req, http.StatusNotFound)
		})

		t.Run("PinLaunch_"+fn, func(t *testing.T) {
			req := newSignedRequest(pri, http.MethodPost, web.PathModule+"/test/hello?action=pin&action=launch&function="+fn, nil)
			resp, content := checkResponse(t, handler, req, http.StatusCreated)

			assert.Equal(t, resp.Header.Get(web.HeaderLocation), web.PathKnownModules+hashHello)

			if s, found := resp.Header[web.HeaderContentType]; found {
				t.Errorf("%q", s)
			}

			Must(t, R(uuid.Parse(resp.Header.Get(web.HeaderInstance))))

			assert.Empty(t, content)

			req = newSignedRequest(pri, http.MethodPost, web.PathKnownModules+hashHello+"?action=unpin", nil)
			checkResponse(t, handler, req, http.StatusOK)
		})
	}

	t.Run("CallExtensionTest", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPost, web.PathModule+"/test/hello?action=call&function=test_ext&action=pin", nil)
		checkResponse(t, handler, req, http.StatusCreated)
	})

	t.Run("HEAD", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodHead, web.PathKnownModules+hashHello, nil)
		resp, content := checkResponse(t, handler, req, http.StatusOK)

		assert.Equal(t, resp.Header.Get(web.HeaderContentType), web.ContentTypeWebAssembly)

		assert.Empty(t, content)
	})

	t.Run("ActionNotImplemented", func(t *testing.T) {
		req := newRequest(http.MethodPost, web.PathModule+"/test/hello?action=bad", nil)
		checkResponse(t, handler, req, http.StatusNotImplemented)

		req = newSignedRequest(pri, http.MethodPost, web.PathModule+"/test/hello?action=bad", nil)
		checkResponse(t, handler, req, http.StatusNotImplemented)
	})
}

func checkInstanceList(t *testing.T, handler http.Handler, pri principalKey, expect any) {
	t.Helper()

	req := newSignedRequest(pri, http.MethodPost, web.PathInstances, nil)
	resp, content := checkResponse(t, handler, req, http.StatusOK)

	assert.Equal(t, resp.Header.Get(web.HeaderContentType), "application/json; charset=utf-8")

	var ids any
	require.NoError(t, json.Unmarshal(content, &ids))
	assert.Equal(t, ids, expect)
}

func checkInstanceStatus(t *testing.T, handler http.Handler, pri principalKey, instID string, expect web.Status) {
	t.Helper()

	req := newSignedRequest(pri, http.MethodPost, web.PathInstances+instID, nil)
	resp, content := checkResponse(t, handler, req, http.StatusOK)

	assert.Equal(t, resp.Header.Get(web.HeaderContentType), "application/json; charset=utf-8")

	var info web.InstanceInfo
	require.NoError(t, json.Unmarshal(content, &info))
	assert.Equal(t, info.Status, expect)
}

func TestResumeSnapshot(t *testing.T) {
	handler := newHandler(t)
	pri := newPrincipalKey()

	test := func(t *testing.T, snapshot []byte) {
		req := newSignedRequest(pri, http.MethodPost, web.PathKnownModules+sha256hex(snapshot)+"?action=launch&log=*", snapshot)
		req.Header.Set(web.HeaderContentType, web.ContentTypeWebAssembly)
		resp, _ := checkResponse(t, handler, req, http.StatusOK)
		id := resp.Header.Get(web.HeaderInstance)

		time.Sleep(time.Second * 3)

		req = newSignedRequest(pri, http.MethodPost, web.PathInstances+id+"?action=kill&action=wait", nil)
		resp, _ = checkResponse(t, handler, req, http.StatusOK)

		checkStatusHeader(t, resp.Header.Get(web.HeaderStatus), web.Status{
			State: web.StateKilled,
		})
	}

	t.Run("amd64", func(t *testing.T) { test(t, wasmSnapshotAMD64) })
	t.Run("arm64", func(t *testing.T) { test(t, wasmSnapshotARM64) })
}

func TestInstance(t *testing.T) {
	handler := newHandler(t)
	pri := newPrincipalKey()

	t.Run("ListEmpty", func(t *testing.T) {
		checkInstanceList(t, handler, pri, map[string]any{})
	})

	var instID string

	{
		req := newSignedRequest(pri, http.MethodPost, web.PathKnownModules+hashHello+"?action=pin&action=launch&function=greet", wasmHello)
		req.Header.Set(web.HeaderContentType, web.ContentTypeWebAssembly)
		resp, _ := checkResponse(t, handler, req, http.StatusCreated)

		instID = resp.Header.Get(web.HeaderInstance)
	}

	t.Run("StatusRunning", func(t *testing.T) {
		checkInstanceStatus(t, handler, pri, instID, web.Status{
			State: web.StateRunning,
		})
	})

	t.Run("ListOne", func(t *testing.T) {
		checkInstanceList(t, handler, pri, map[string]any{
			"instances": []any{
				map[string]any{
					"instance": instID,
					"module":   hashHello,
					"status": map[string]any{
						"state": web.StateRunning,
					},
				},
			},
		})
	})

	t.Run("DeleteFail", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPost, web.PathInstances+instID+"?action=delete", nil)
		checkResponse(t, handler, req, http.StatusConflict)
	})

	t.Run("IO", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPost, web.PathInstances+instID+"?action=io", nil)
		req.Header.Set(web.HeaderTE, web.TETrailers)
		resp, content := checkResponse(t, handler, req, http.StatusOK)

		if s, found := resp.Header[web.HeaderContentType]; found {
			t.Errorf("%q", s)
		}

		assert.Equal(t, string(content), "hello, world\n")

		if !(resp.Trailer.Get(web.HeaderStatus) == `{"state":"RUNNING"}` || resp.Trailer.Get(web.HeaderStatus) == `{"state":"HALTED"}`) || len(resp.Trailer) != 1 {
			t.Errorf("trailer: %v", resp.Trailer)
		}
	})

	t.Run("Wait", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPost, web.PathInstances+instID+"?action=wait", nil)
		resp, _ := checkResponse(t, handler, req, http.StatusOK)

		checkStatusHeader(t, resp.Header.Get(web.HeaderStatus), web.Status{
			State: web.StateHalted,
		})
	})

	t.Run("StatusHalted", func(t *testing.T) {
		checkInstanceStatus(t, handler, pri, instID, web.Status{
			State: web.StateHalted,
		})
	})

	t.Run("ActionNotImplemented", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPost, web.PathInstances+instID+"?action=bad", nil)
		checkResponse(t, handler, req, http.StatusNotImplemented)
	})

	t.Run("Delete", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPost, web.PathInstances+instID+"?action=delete", nil)
		checkResponse(t, handler, req, http.StatusOK)
	})

	t.Run("ListEmptyAgain", func(t *testing.T) {
		checkInstanceList(t, handler, pri, map[string]any{})
	})
}

func TestInstanceMultiIO(t *testing.T) {
	handler := newHandler(t)
	pri := newPrincipalKey()

	var instID string

	{
		req := newSignedRequest(pri, http.MethodPost, web.PathKnownModules+hashHello+"?action=pin&action=launch&function=multi", wasmHello)
		req.Header.Set(web.HeaderContentType, web.ContentTypeWebAssembly)
		resp, _ := checkResponse(t, handler, req, http.StatusCreated)

		instID = resp.Header.Get(web.HeaderInstance)
	}

	done := make(chan struct{}, 10)

	for i := 0; i < cap(done); i++ {
		go func() {
			defer func() { done <- struct{}{} }()

			req := newSignedRequest(pri, http.MethodPost, web.PathInstances+instID+"?action=io", nil)
			req.Header.Set(web.HeaderTE, web.TETrailers)
			resp, content := checkResponse(t, handler, req, http.StatusOK)

			if s, found := resp.Header[web.HeaderContentType]; found {
				t.Errorf("%q", s)
			}

			assert.Equal(t, string(content), "hello, world\n")

			checkStatusHeader(t, resp.Trailer.Get(web.HeaderStatus), web.Status{
				State: web.StateRunning,
			})

			assert.Equal(t, len(resp.Trailer), 1)
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
		req := newSignedRequest(pri, http.MethodPost, web.PathKnownModules+hashHello+"?action=launch&function=multi", wasmHello)
		req.Header.Set(web.HeaderContentType, web.ContentTypeWebAssembly)
		resp, _ := checkResponse(t, handler, req, http.StatusOK)

		instID = resp.Header.Get(web.HeaderInstance)
	}

	t.Run("KillWait", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPost, web.PathInstances+instID+"?action=kill&action=wait", nil)
		resp, _ := checkResponse(t, handler, req, http.StatusOK)

		checkStatusHeader(t, resp.Header.Get(web.HeaderStatus), web.Status{
			State: web.StateKilled,
		})
	})

	t.Run("Status", func(t *testing.T) {
		checkInstanceStatus(t, handler, pri, instID, web.Status{
			State: web.StateKilled,
		})
	})
}

func TestInstanceSuspend(t *testing.T) {
	handler := newHandler(t)
	pri := newPrincipalKey()

	var instID string

	{
		req := newSignedRequest(pri, http.MethodPost, web.PathKnownModules+hashSuspend+"?action=launch&function=loop&log=*", wasmSuspend)
		req.Header.Set(web.HeaderContentType, web.ContentTypeWebAssembly)
		resp, _ := checkResponse(t, handler, req, http.StatusOK)

		instID = resp.Header.Get(web.HeaderInstance)
	}

	time.Sleep(time.Second / 3)

	t.Run("Suspend", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPost, web.PathInstances+instID+"?action=suspend", nil)
		resp, _ := checkResponse(t, handler, req, http.StatusOK)

		assert.NotContains(t, resp.Header, web.HeaderStatus)
	})

	t.Run("Wait", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPost, web.PathInstances+instID+"?action=wait", nil)
		resp, _ := checkResponse(t, handler, req, http.StatusOK)

		checkStatusHeader(t, resp.Header.Get(web.HeaderStatus), web.Status{
			State: web.StateSuspended,
		})
	})

	var snapshot []byte

	t.Run("Snapshot", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPost, web.PathInstances+instID+"?action=snapshot", nil)
		resp, _ := checkResponse(t, handler, req, http.StatusCreated)

		location := resp.Header.Get(web.HeaderLocation)
		assert.NotEmpty(t, location)

		req = newSignedRequest(pri, http.MethodGet, location, nil)
		_, snapshot = checkResponse(t, handler, req, http.StatusOK)

		if false {
			f := Must(t, R(os.Create("/tmp/snapshot.wasm")))
			defer f.Close()
			Must(t, R(f.Write(snapshot)))
		}

		config := &wag.Config{ImportResolver: new(abi.ImportResolver)}
		Must(t, R(wag.Compile(config, bytes.NewReader(snapshot), abi.Library())))
	})

	t.Run("ResumeFunction", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPost, web.PathInstances+instID+"?action=resume&function=loop", nil)
		checkResponse(t, handler, req, http.StatusConflict)
	})

	t.Run("Resume", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPost, web.PathInstances+instID+"?action=resume&log=*", nil)
		checkResponse(t, handler, req, http.StatusOK)

		time.Sleep(time.Second / 3)

		checkInstanceStatus(t, handler, pri, instID, web.Status{
			State: web.StateRunning,
		})

		time.Sleep(time.Second / 3)

		req = newSignedRequest(pri, http.MethodPost, web.PathInstances+instID+"?action=suspend", nil)
		checkResponse(t, handler, req, http.StatusOK)

		req = newSignedRequest(pri, http.MethodPost, web.PathInstances+instID+"?action=wait", nil)
		resp, _ := checkResponse(t, handler, req, http.StatusOK)

		checkStatusHeader(t, resp.Header.Get(web.HeaderStatus), web.Status{
			State: web.StateSuspended,
		})
	})

	handler2 := newHandler(t)

	t.Run("Restore", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPost, web.PathKnownModules+sha256hex(snapshot)+"?action=launch&log=*", snapshot)
		req.Header.Set(web.HeaderContentType, web.ContentTypeWebAssembly)
		resp, _ := checkResponse(t, handler2, req, http.StatusOK)
		restoredID := resp.Header.Get(web.HeaderInstance)

		time.Sleep(time.Second / 3)

		req = newSignedRequest(pri, http.MethodPost, web.PathInstances+restoredID+"?action=suspend", nil)
		checkResponse(t, handler2, req, http.StatusOK)

		req = newSignedRequest(pri, http.MethodPost, web.PathInstances+restoredID+"?action=wait", nil)
		resp, _ = checkResponse(t, handler2, req, http.StatusOK)

		checkStatusHeader(t, resp.Header.Get(web.HeaderStatus), web.Status{
			State: web.StateSuspended,
		})
	})
}

func TestInstanceTerminated(t *testing.T) {
	handler := newHandler(t)
	pri := newPrincipalKey()

	var instID string

	{
		req := newSignedRequest(pri, http.MethodPost, web.PathKnownModules+hashHello+"?action=launch&function=fail", wasmHello)
		req.Header.Set(web.HeaderContentType, web.ContentTypeWebAssembly)
		resp, _ := checkResponse(t, handler, req, http.StatusOK)

		instID = resp.Header.Get(web.HeaderInstance)
	}

	t.Run("Wait", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPost, web.PathInstances+instID+"?action=wait", nil)
		resp, _ := checkResponse(t, handler, req, http.StatusOK)

		checkStatusHeader(t, resp.Header.Get(web.HeaderStatus), web.Status{
			State:  web.StateTerminated,
			Result: 1,
		})
	})

	t.Run("Snapshot", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPost, web.PathInstances+instID+"?action=snapshot", nil)
		resp, _ := checkResponse(t, handler, req, http.StatusCreated)

		location := resp.Header.Get(web.HeaderLocation)
		assert.NotEmpty(t, location)

		req = newSignedRequest(pri, http.MethodGet, location, nil)
		_, snapshot := checkResponse(t, handler, req, http.StatusOK)

		if false {
			f := Must(t, R(os.Create("/tmp/snapshot.wasm")))
			defer f.Close()
			Must(t, R(f.Write(snapshot)))
		}

		var final bool

		loaders := map[string]section.CustomContentLoader{
			wasm.SectionSnapshot: func(_ string, r section.Reader, _ uint32) error {
				snap, _, err := wasm.ReadSnapshotSection(r)
				if err != nil {
					return err
				}
				final = snap.GetFinal()
				return nil
			},
		}

		c := compile.Config{CustomSectionLoader: section.CustomLoader(loaders)}
		r := compile.NewLoader(bytes.NewReader(snapshot))
		m := Must(t, R(compile.LoadInitialSections(&compile.ModuleConfig{Config: c}, r)))
		binding.BindImports(&m, new(abi.ImportResolver))
		require.NoError(t, compile.LoadCodeSection(&compile.CodeConfig{Config: c}, r, m, abi.Library()))
		require.NoError(t, compile.LoadDataSection(&compile.DataConfig{Config: c}, r, m))
		require.NoError(t, compile.LoadCustomSections(&c, r))
		assert.True(t, final, "snapshot section did not have final flag set")
	})
}

// TODO: WebSocket tests
