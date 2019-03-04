// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gate_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"github.com/tsavola/gate/runtime/abi"
	"github.com/tsavola/gate/server"
	"github.com/tsavola/gate/server/state"
	"github.com/tsavola/gate/server/state/sql"
	"github.com/tsavola/gate/server/webserver"
	"github.com/tsavola/gate/service"
	"github.com/tsavola/gate/service/origin"
	"github.com/tsavola/gate/service/plugin"
	"github.com/tsavola/gate/webapi"
	"github.com/tsavola/gate/webapi/authorization"
	"github.com/tsavola/wag"
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

var accessTracker state.AccessTracker

func init() {
	db, err := sql.Open(context.Background(), sql.Config{
		Driver: "sqlite3",
		DSN:    "file::memory:?cache=shared",
	})
	if err != nil {
		panic(err)
	}

	accessTracker = db
}

func newServices() func() server.InstanceServices {
	registry := new(service.Registry)

	plugins, err := plugin.OpenAll("lib/gate/plugin")
	if err != nil {
		panic(err)
	}

	err = plugins.InitServices(service.Config{
		Registry: registry,
	})
	if err != nil {
		panic(err)
	}

	return func() server.InstanceServices {
		connector := origin.New(nil)
		r := registry.Clone()
		r.Register(connector)
		return server.NewInstanceServices(r, connector)
	}
}

func newServer(ctx context.Context) *server.Server {
	config := &server.Config{
		Executor:     newExecutor(ctx, nil).Executor,
		AccessPolicy: server.NewPublicAccess(newServices()),
		Debug:        os.Stdout,
	}

	return server.New(ctx, config)
}

func newHandler(ctx context.Context) http.Handler {
	config := &webserver.Config{
		Server:        newServer(ctx),
		Authority:     "test",
		AccessState:   accessTracker,
		ModuleSources: map[string]server.Source{"/test": helloSource{}},
	}

	h := webserver.NewHandler(ctx, "/", config)
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
		Aud:   []string{"no", "https://test/gate/v0"},
		Nonce: strconv.Itoa(rand.Int()),
	}))
	return
}

func checkResponse(t *testing.T, handler http.Handler, req *http.Request, expectStatusCode int,
) (resp *http.Response, content []byte) {
	t.Helper()

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	resp = w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != expectStatusCode {
		t.Fatalf("response status: %d %q", resp.StatusCode, resp.Status)
	}

	content, err := ioutil.ReadAll(resp.Body)
	// t.Logf("response content: %q", content)
	if err != nil {
		t.Fatalf("response content error: %v", err)
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

func TestRoot(t *testing.T) {
	ctx := context.Background()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp, content := checkResponse(t, newHandler(ctx), req, http.StatusOK)

	if x := resp.Header.Get(webapi.HeaderContentType); x != "application/json; charset=utf-8" {
		t.Error(x)
	}

	var versions interface{}

	if err := json.Unmarshal(content, &versions); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(versions, []interface{}{
		strings.TrimLeft(webapi.Path, "/"),
	}) {
		t.Errorf("%#v", versions)
	}
}

func TestRoot404(t *testing.T) {
	ctx := context.Background()
	req := httptest.NewRequest(http.MethodGet, "/foo", nil)
	resp, content := checkResponse(t, newHandler(ctx), req, http.StatusNotFound)

	if x := resp.Header.Get(webapi.HeaderContentType); x != "text/plain; charset=utf-8" {
		t.Error(x)
	}

	if string(content) != "not found\n" {
		t.Error(content)
	}
}

func TestAPI(t *testing.T) {
	ctx := context.Background()
	req := httptest.NewRequest(http.MethodGet, webapi.Path, nil)
	resp, content := checkResponse(t, newHandler(ctx), req, http.StatusOK)

	if x := resp.Header.Get(webapi.HeaderContentType); x != "application/json; charset=utf-8" {
		t.Error(x)
	}

	var info interface{}

	if err := json.Unmarshal(content, &info); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(info, map[string]interface{}{
		"runtime": map[string]interface{}{
			"max_abi_version": float64(abi.MaxVersion),
			"min_abi_version": float64(abi.MinVersion),
		},
	}) {
		t.Errorf("%#v", info)
	}
}

func TestModuleSourceList(t *testing.T) {
	ctx := context.Background()
	req := httptest.NewRequest(http.MethodGet, webapi.PathModules, nil)
	resp, content := checkResponse(t, newHandler(ctx), req, http.StatusOK)

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
	ctx := context.Background()
	handler := newHandler(ctx)
	pri := newPrincipalKey()

	t.Run("ListEmpty", func(t *testing.T) {
		checkModuleList(t, handler, pri, map[string]interface{}{
			"modules": []interface{}{},
		})
	})

	t.Run("Put", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPut, webapi.PathModuleRefs+hashHello, wasmHello)
		req.Header.Set(webapi.HeaderContentType, webapi.ContentTypeWebAssembly)
		resp, content := checkResponse(t, handler, req, http.StatusCreated)

		if _, found := resp.Header[webapi.HeaderContentType]; found {
			t.Fail()
		}

		if len(content) != 0 {
			t.Error(content)
		}
	})

	t.Run("ListOne", func(t *testing.T) {
		checkModuleList(t, handler, pri, map[string]interface{}{
			"modules": []interface{}{
				map[string]interface{}{
					"key": hashHello,
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
					"key": hashHello,
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
		{"main", "hello, world\n"},
		{"twice", "hello, world\nhello, world\n"},
	} {
		fn := spec[0]
		expect := spec[1]

		t.Run("Call"+strings.Title(fn), func(t *testing.T) {
			req := newSignedRequest(pri, http.MethodPost, webapi.PathModuleRefs+hashHello+"?action=call&function="+fn, nil)
			resp, content := checkResponse(t, handler, req, http.StatusOK)

			if _, found := resp.Header[webapi.HeaderContentType]; found {
				t.Fail()
			}

			if _, err := uuid.Parse(resp.Header.Get(webapi.HeaderInstance)); err != nil {
				t.Error(err)
			}

			if string(content) != expect {
				t.Errorf("%q", content)
			}

			checkStatusHeader(t, resp.Trailer.Get(webapi.HeaderStatus), webapi.Status{
				State: "terminated",
			})

			if len(resp.Trailer) != 1 {
				t.Errorf("trailer: %v", resp.Trailer)
			}
		})

		t.Run("Launch"+strings.Title(fn), func(t *testing.T) {
			req := newSignedRequest(pri, http.MethodPost, webapi.PathModuleRefs+hashHello+"?action=launch&function="+fn, nil)
			resp, content := checkResponse(t, handler, req, http.StatusOK)

			if _, found := resp.Header[webapi.HeaderContentType]; found {
				t.Fail()
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

		if _, found := resp.Header[webapi.HeaderContentType]; found {
			t.Fail()
		}

		if len(content) != 0 {
			t.Error(content)
		}
	})

	t.Run("UnrefNotFound", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPost, webapi.PathModuleRefs+hashHello+"?action=unref", nil)
		checkResponse(t, handler, req, http.StatusNotFound)
	})

	t.Run("ListEmptyAgain", func(t *testing.T) {
		checkModuleList(t, handler, pri, map[string]interface{}{
			"modules": []interface{}{},
		})
	})

	t.Run("LaunchContent", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPost, webapi.PathModuleRefs+hashHello+"?action=launch", wasmHello)
		req.Header.Set(webapi.HeaderContentType, webapi.ContentTypeWebAssembly)
		resp, content := checkResponse(t, handler, req, http.StatusCreated)

		if x := resp.Header.Get(webapi.HeaderLocation); x != webapi.PathModuleRefs+hashHello {
			t.Error(x)
		}

		if _, found := resp.Header[webapi.HeaderContentType]; found {
			t.Fail()
		}

		if _, err := uuid.Parse(resp.Header.Get(webapi.HeaderInstance)); err != nil {
			t.Error(err)
		}

		if len(content) != 0 {
			t.Error(content)
		}
	})

	t.Run("ListOneAgain", func(t *testing.T) {
		checkModuleList(t, handler, pri, map[string]interface{}{
			"modules": []interface{}{
				map[string]interface{}{
					"key": hashHello,
				},
			},
		})
	})
}

func TestModuleSource(t *testing.T) {
	ctx := context.Background()
	handler := newHandler(ctx)
	pri := newPrincipalKey()

	for _, spec := range [][2]string{
		{"", ""},
		{"main", "hello, world\n"},
		{"twice", "hello, world\nhello, world\n"},
	} {
		fn := spec[0]
		expect := spec[1]

		t.Run("AnonCall"+strings.Title(fn), func(t *testing.T) {
			req := newRequest(http.MethodPost, webapi.PathModule+"/test/hello?action=call&function="+fn, nil)
			resp, content := checkResponse(t, handler, req, http.StatusOK)

			if _, found := resp.Header[webapi.HeaderContentType]; found {
				t.Fail()
			}
			if _, found := resp.Header[webapi.HeaderLocation]; found {
				t.Fail()
			}
			if _, found := resp.Header[webapi.HeaderInstance]; found {
				t.Fail()
			}

			if string(content) != expect {
				t.Errorf("%q", content)
			}

			checkStatusHeader(t, resp.Trailer.Get(webapi.HeaderStatus), webapi.Status{
				State: "terminated",
			})

			if len(resp.Trailer) != 1 {
				t.Errorf("trailer: %v", resp.Trailer)
			}
		})

		t.Run("Call"+strings.Title(fn), func(t *testing.T) {
			req := newSignedRequest(pri, http.MethodPost, webapi.PathModule+"/test/hello?action=call&function="+fn, nil)
			resp, content := checkResponse(t, handler, req, http.StatusCreated)

			if x := resp.Header.Get(webapi.HeaderLocation); x != webapi.PathModuleRefs+hashHello {
				t.Error(x)
			}

			if _, found := resp.Header[webapi.HeaderContentType]; found {
				t.Fail()
			}

			if _, err := uuid.Parse(resp.Header.Get(webapi.HeaderInstance)); err != nil {
				t.Error(err)
			}

			if string(content) != expect {
				t.Errorf("%q", content)
			}

			checkStatusHeader(t, resp.Trailer.Get(webapi.HeaderStatus), webapi.Status{
				State: "terminated",
			})

			if len(resp.Trailer) != 1 {
				t.Errorf("trailer: %v", resp.Trailer)
			}
		})

		t.Run("Launch"+strings.Title(fn), func(t *testing.T) {
			req := newSignedRequest(pri, http.MethodPost, webapi.PathModule+"/test/hello?action=launch&function="+fn, nil)
			resp, content := checkResponse(t, handler, req, http.StatusCreated)

			if x := resp.Header.Get(webapi.HeaderLocation); x != webapi.PathModuleRefs+hashHello {
				t.Error(x)
			}

			if _, found := resp.Header[webapi.HeaderContentType]; found {
				t.Fail()
			}

			if _, err := uuid.Parse(resp.Header.Get(webapi.HeaderInstance)); err != nil {
				t.Error(err)
			}

			if len(content) != 0 {
				t.Error(content)
			}
		})
	}

	t.Run("CallPluginTest", func(t *testing.T) {
		req := newSignedRequest(pri, http.MethodPost, webapi.PathModule+"/test/hello?action=call&function=test_plugin", nil)
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

	if _, found := resp.Header[webapi.HeaderContentType]; found {
		t.Fail()
	}

	if len(content) != 0 {
		t.Error(content)
	}

	checkStatusHeader(t, resp.Header.Get(webapi.HeaderStatus), expect)
}

func TestInstance(t *testing.T) {
	ctx := context.Background()
	handler := newHandler(ctx)
	pri := newPrincipalKey()

	t.Run("ListEmpty", func(t *testing.T) {
		checkInstanceList(t, handler, pri, map[string]interface{}{
			"instances": []interface{}{},
		})
	})

	{
		var instID string

		{
			req := newSignedRequest(pri, http.MethodPost, webapi.PathModuleRefs+hashHello+"?action=launch&function=main", wasmHello)
			req.Header.Set(webapi.HeaderContentType, webapi.ContentTypeWebAssembly)
			resp, _ := checkResponse(t, handler, req, http.StatusCreated)

			instID = resp.Header.Get(webapi.HeaderInstance)
		}

		t.Run("StatusRunning", func(t *testing.T) {
			checkInstanceStatus(t, handler, pri, instID, webapi.Status{
				State: "running",
			})
		})

		t.Run("ListOne", func(t *testing.T) {
			checkInstanceList(t, handler, pri, map[string]interface{}{
				"instances": []interface{}{
					map[string]interface{}{
						"instance": instID,
						"status": map[string]interface{}{
							"state": "running",
						},
					},
				},
			})
		})

		t.Run("IO", func(t *testing.T) {
			req := newSignedRequest(pri, http.MethodPost, webapi.PathInstances+instID+"?action=io", nil)
			resp, content := checkResponse(t, handler, req, http.StatusOK)

			if _, found := resp.Header[webapi.HeaderContentType]; found {
				t.Fail()
			}

			if string(content) != "hello, world\n" {
				t.Errorf("%q", content)
			}

			if !(resp.Trailer.Get(webapi.HeaderStatus) == `{"state":"running"}` || resp.Trailer.Get(webapi.HeaderStatus) == `{"state":"terminated"}`) || len(resp.Trailer) != 1 {
				t.Errorf("trailer: %v", resp.Trailer)
			}
		})

		t.Run("Wait", func(t *testing.T) {
			req := newSignedRequest(pri, http.MethodPost, webapi.PathInstances+instID+"?action=wait", nil)
			resp, _ := checkResponse(t, handler, req, http.StatusNoContent)

			checkStatusHeader(t, resp.Header.Get(webapi.HeaderStatus), webapi.Status{
				State: "terminated",
			})
		})

		for i := 0; i < 3; i++ {
			t.Run(fmt.Sprintf("StatusTerminated%d", i), func(t *testing.T) {
				t.Parallel()
				checkInstanceStatus(t, handler, pri, instID, webapi.Status{
					State: "terminated",
				})
			})
		}
	}

	{
		var instID string

		{
			req := newSignedRequest(pri, http.MethodPost, webapi.PathModuleRefs+hashHello+"?action=launch&function=multi", wasmHello)
			req.Header.Set(webapi.HeaderContentType, webapi.ContentTypeWebAssembly)
			resp, _ := checkResponse(t, handler, req, http.StatusCreated)

			instID = resp.Header.Get(webapi.HeaderInstance)
		}

		t.Run("MultiIO", func(t *testing.T) {
			done := make(chan struct{}, 10)

			for i := 0; i < cap(done); i++ {
				go func() {
					defer func() { done <- struct{}{} }()

					req := newSignedRequest(pri, http.MethodPost, webapi.PathInstances+instID+"?action=io", nil)
					resp, content := checkResponse(t, handler, req, http.StatusOK)

					if _, found := resp.Header[webapi.HeaderContentType]; found {
						t.Fail()
					}

					if string(content) != "hello, world\n" {
						t.Errorf("%q", content)
					}

					checkStatusHeader(t, resp.Trailer.Get(webapi.HeaderStatus), webapi.Status{
						State: "running",
					})

					if len(resp.Trailer) != 1 {
						t.Errorf("trailer: %v", resp.Trailer)
					}
				}()
			}

			for i := 0; i < cap(done); i++ {
				<-done
			}
		})
	}

	{
		var instID string

		{
			req := newSignedRequest(pri, http.MethodPost, webapi.PathModuleRefs+hashSuspend+"?action=launch&function=main", wasmSuspend)
			req.Header.Set(webapi.HeaderContentType, webapi.ContentTypeWebAssembly)
			resp, _ := checkResponse(t, handler, req, http.StatusCreated)

			instID = resp.Header.Get(webapi.HeaderInstance)
		}

		t.Run("Suspend", func(t *testing.T) {
			req := newSignedRequest(pri, http.MethodPost, webapi.PathInstances+instID+"?action=suspend", nil)
			resp, _ := checkResponse(t, handler, req, http.StatusNoContent)

			checkStatusHeader(t, resp.Header.Get(webapi.HeaderStatus), webapi.Status{
				State: "suspended",
			})
		})

		t.Run("StatusSuspended", func(t *testing.T) {
			checkInstanceStatus(t, handler, pri, instID, webapi.Status{
				State: "suspended",
			})
		})

		var snapshotKey string
		var snapshot []byte

		t.Run("Snapshot", func(t *testing.T) {
			req := newSignedRequest(pri, http.MethodPost, webapi.PathInstances+instID+"?action=snapshot", nil)
			resp, _ := checkResponse(t, handler, req, http.StatusCreated)

			location := resp.Header.Get(webapi.HeaderLocation)
			if location == "" {
				t.Fatal("no module location")
			}

			snapshotKey = path.Base(location)

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

			if _, err := wag.Compile(nil, bytes.NewReader(snapshot), abi.Imports); err != nil {
				t.Error(err)
			}
		})

		t.Run("ListSuspendedModule", func(t *testing.T) {
			req := newSignedRequest(pri, http.MethodGet, webapi.PathModuleRefs, nil)
			_, content := checkResponse(t, handler, req, http.StatusOK)

			var refs interface{}

			if err := json.Unmarshal(content, &refs); err != nil {
				t.Fatal(err)
			}

			var found bool

			for _, x := range refs.(map[string]interface{})["modules"].([]interface{}) {
				ref := x.(map[string]interface{})
				if ref["key"].(string) == snapshotKey {
					if !ref["suspended"].(bool) {
						t.Error("snapshot module is not suspended")
					}
					found = true
					break
				}
			}

			if !found {
				t.Error("snapshot module key not found")
			}
		})

		t.Run("Restore", func(t *testing.T) {
			req := newSignedRequest(pri, http.MethodPost, webapi.PathModuleRefs+sha384(snapshot)+"?action=launch", snapshot)
			req.Header.Set(webapi.HeaderContentType, webapi.ContentTypeWebAssembly)
			resp, _ := checkResponse(t, handler, req, http.StatusCreated)

			restoredID := resp.Header.Get(webapi.HeaderInstance)

			req = newSignedRequest(pri, http.MethodPost, webapi.PathInstances+restoredID+"?action=suspend", nil)
			resp, _ = checkResponse(t, handler, req, http.StatusNoContent)

			req = newSignedRequest(pri, http.MethodPost, webapi.PathInstances+restoredID+"?action=wait", nil)
			resp, _ = checkResponse(t, handler, req, http.StatusNoContent)

			checkStatusHeader(t, resp.Header.Get(webapi.HeaderStatus), webapi.Status{
				State: "suspended",
			})
		})
	}
}

// TODO: WebSocket tests
