// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webserver

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
	"github.com/gorilla/handlers"
	_ "github.com/mattn/go-sqlite3"
	"github.com/tsavola/gate/internal/test/runtimeutil"
	"github.com/tsavola/gate/runtime"
	"github.com/tsavola/gate/runtime/abi"
	"github.com/tsavola/gate/server"
	"github.com/tsavola/gate/server/state"
	"github.com/tsavola/gate/server/state/sql"
	"github.com/tsavola/gate/webapi"
	"github.com/tsavola/wag"
	"github.com/tsavola/wag/wa"
)

const testAccessLog = false

var testAccessTracker state.AccessTracker

func init() {
	db, err := sql.Open(context.Background(), sql.Config{
		Driver: "sqlite3",
		DSN:    "file::memory:?cache=shared",
	})
	if err != nil {
		panic(err)
	}

	testAccessTracker = db
}

func newTestHandler(ctx context.Context) http.Handler {
	runtimeConfig := &runtime.Config{
		LibDir: "../../lib/gate/runtime",
	}

	serverConfig := &server.Config{
		Executor:     runtimeutil.NewExecutor(ctx, runtimeConfig).Executor,
		AccessPolicy: server.NewPublicAccess(newTestServices()),
		Debug:        os.Stderr, // TODO: os.Stdout,
	}

	webConfig := &Config{
		Server:      server.New(ctx, serverConfig),
		Authority:   "test",
		AccessState: testAccessTracker,
		ModuleSources: map[string]server.Source{
			"/test": testSource{},
		},
	}

	handler := NewHandler(ctx, "/", webConfig)

	if testAccessLog {
		handler = handlers.LoggingHandler(os.Stdout, handler)
	}

	return handler
}

func newAnonTestRequest(key *testKey, method, path string, content []byte) (req *http.Request) {
	var body io.ReadCloser
	if content != nil {
		body = ioutil.NopCloser(bytes.NewReader(content))
	}
	req = httptest.NewRequest(method, path, body)
	req.ContentLength = int64(len(content))
	return
}

func newSignedTestRequest(key *testKey, method, path string, content []byte) (req *http.Request) {
	req = newAnonTestRequest(key, method, path, content)
	req.Header.Set(webapi.HeaderAuthorization, key.authorization(&webapi.Claims{
		Exp:   time.Now().Add(time.Minute).Unix(),
		Aud:   []string{"no", "https://test/gate/v0"},
		Nonce: strconv.Itoa(rand.Int()),
	}))
	return
}

func doTest(t *testing.T, handler http.Handler, req *http.Request) (resp *http.Response, content []byte) {
	t.Helper()

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	resp = w.Result()
	content, err := ioutil.ReadAll(resp.Body)
	// t.Logf("response content: %q", content)
	if err != nil {
		t.Fatal(err)
	}
	return
}

func testStatusResponse(t *testing.T, statusHeader string, expect webapi.Status) {
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
	resp, content := doTest(t, newTestHandler(ctx), req)

	if resp.StatusCode != http.StatusOK {
		t.Fatal(resp.Status)
	}

	if x := resp.Header.Get(webapi.HeaderContentType); x != contentTypeJSON {
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
	resp, content := doTest(t, newTestHandler(ctx), req)

	if resp.StatusCode != http.StatusNotFound {
		t.Fatal(resp.Status)
	}

	if x := resp.Header.Get(webapi.HeaderContentType); x != contentTypeText {
		t.Error(x)
	}

	if string(content) != "not found\n" {
		t.Error(content)
	}
}

func TestAPI(t *testing.T) {
	ctx := context.Background()
	req := httptest.NewRequest(http.MethodGet, webapi.Path, nil)
	resp, content := doTest(t, newTestHandler(ctx), req)

	if resp.StatusCode != http.StatusOK {
		t.Fatal(resp.Status)
	}

	if x := resp.Header.Get(webapi.HeaderContentType); x != contentTypeJSON {
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
	resp, content := doTest(t, newTestHandler(ctx), req)

	if resp.StatusCode != http.StatusOK {
		t.Fatal(resp.Status)
	}

	if x := resp.Header.Get(webapi.HeaderContentType); x != contentTypeJSON {
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

func TestModuleRef(t *testing.T) {
	ctx := context.Background()
	key := newTestKey()
	handler := newTestHandler(ctx)
	path := webapi.PathModuleRefs

	testList := func(t *testing.T, expect interface{}) {
		t.Helper()

		req := newSignedTestRequest(key, http.MethodGet, path, nil)
		resp, content := doTest(t, handler, req)

		if resp.StatusCode != http.StatusOK {
			t.Fatal(resp.Status)
		}

		if x := resp.Header.Get(webapi.HeaderContentType); x != contentTypeJSON {
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

	t.Run("ListEmpty", func(t *testing.T) {
		testList(t, map[string]interface{}{
			"modules": []interface{}{},
		})
	})

	t.Run("Put", func(t *testing.T) {
		req := newSignedTestRequest(key, http.MethodPut, path+testHashHello, testProgHello)
		req.Header.Set(webapi.HeaderContentType, webapi.ContentTypeWebAssembly)
		resp, content := doTest(t, handler, req)

		if resp.StatusCode != http.StatusCreated {
			t.Fatal(resp.Status)
		}

		if _, found := resp.Header[webapi.HeaderContentType]; found {
			t.Fail()
		}

		if len(content) != 0 {
			t.Error(content)
		}
	})

	t.Run("ListOne", func(t *testing.T) {
		testList(t, map[string]interface{}{
			"modules": []interface{}{
				map[string]interface{}{
					"key": testHashHello,
				},
			},
		})
	})

	t.Run("PutWrongHash", func(t *testing.T) {
		req := newSignedTestRequest(key, http.MethodPut, path+sha384([]byte("asdf")), testProgHello)
		req.Header.Set(webapi.HeaderContentType, webapi.ContentTypeWebAssembly)
		resp, _ := doTest(t, handler, req)

		if resp.StatusCode != http.StatusBadRequest {
			t.Fatal(resp.Status)
		}
	})

	t.Run("ListStillOne", func(t *testing.T) {
		testList(t, map[string]interface{}{
			"modules": []interface{}{
				map[string]interface{}{
					"key": testHashHello,
				},
			},
		})
	})

	t.Run("Get", func(t *testing.T) {
		req := newSignedTestRequest(key, http.MethodGet, path+testHashHello, nil)
		resp, content := doTest(t, handler, req)

		if resp.StatusCode != http.StatusOK {
			t.Fatal(resp.Status)
		}

		if x := resp.Header.Get(webapi.HeaderContentType); x != webapi.ContentTypeWebAssembly {
			t.Error(x)
		}

		if !bytes.Equal(content, testProgHello) {
			t.Error(content)
		}
	})

	t.Run("GetNotFound", func(t *testing.T) {
		req := newSignedTestRequest(key, http.MethodGet, path+"3R_g4HTkvIb0sx8-ppwrrJRu3T6rT5mpA3SvAmifGMmGzYB7xIAMbS9qmax5WigT", nil)
		resp, content := doTest(t, handler, req)

		if resp.StatusCode != http.StatusNotFound {
			t.Fatal(resp.Status)
		}

		if x := resp.Header.Get(webapi.HeaderContentType); x != contentTypeText {
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
			req := newSignedTestRequest(key, http.MethodPost, path+testHashHello+"?action=call&function="+fn, nil)
			resp, content := doTest(t, handler, req)

			if resp.StatusCode != http.StatusOK {
				t.Fatal(resp.Status)
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

			testStatusResponse(t, resp.Trailer.Get(webapi.HeaderStatus), webapi.Status{
				State: "terminated",
			})

			if len(resp.Trailer) != 1 {
				t.Errorf("trailer: %v", resp.Trailer)
			}
		})

		t.Run("Launch"+strings.Title(fn), func(t *testing.T) {
			req := newSignedTestRequest(key, http.MethodPost, path+testHashHello+"?action=launch&function="+fn, nil)
			resp, content := doTest(t, handler, req)

			if resp.StatusCode != http.StatusOK {
				t.Fatal(resp.Status)
			}

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
		req := newSignedTestRequest(key, http.MethodPost, path+testHashHello+"?action=unref", nil)
		resp, content := doTest(t, handler, req)

		if resp.StatusCode != http.StatusNoContent {
			t.Fatal(resp.Status)
		}

		if _, found := resp.Header[webapi.HeaderContentType]; found {
			t.Fail()
		}

		if len(content) != 0 {
			t.Error(content)
		}
	})

	t.Run("UnrefNotFound", func(t *testing.T) {
		req := newSignedTestRequest(key, http.MethodPost, path+testHashHello+"?action=unref", nil)
		resp, _ := doTest(t, handler, req)

		if resp.StatusCode != http.StatusNotFound {
			t.Fatal(resp.Status)
		}
	})

	t.Run("ListEmptyAgain", func(t *testing.T) {
		testList(t, map[string]interface{}{
			"modules": []interface{}{},
		})
	})

	t.Run("LaunchContent", func(t *testing.T) {
		req := newSignedTestRequest(key, http.MethodPost, path+testHashHello+"?action=launch", testProgHello)
		req.Header.Set(webapi.HeaderContentType, webapi.ContentTypeWebAssembly)
		resp, content := doTest(t, handler, req)

		if resp.StatusCode != http.StatusCreated {
			t.Fatal(resp.Status)
		}

		if x := resp.Header.Get(webapi.HeaderLocation); x != path+testHashHello {
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
		testList(t, map[string]interface{}{
			"modules": []interface{}{
				map[string]interface{}{
					"key": testHashHello,
				},
			},
		})
	})
}

func TestModuleSource(t *testing.T) {
	ctx := context.Background()
	key := newTestKey()
	handler := newTestHandler(ctx)
	path := webapi.PathModule + "/test/hello"

	for _, spec := range [][2]string{
		{"", ""},
		{"main", "hello, world\n"},
		{"twice", "hello, world\nhello, world\n"},
	} {
		fn := spec[0]
		expect := spec[1]

		t.Run("AnonCall"+strings.Title(fn), func(t *testing.T) {
			req := newAnonTestRequest(key, http.MethodPost, path+"?action=call&function="+fn, nil)
			resp, content := doTest(t, handler, req)

			if resp.StatusCode != http.StatusOK {
				t.Fatal(resp.Status)
			}

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

			testStatusResponse(t, resp.Trailer.Get(webapi.HeaderStatus), webapi.Status{
				State: "terminated",
			})

			if len(resp.Trailer) != 1 {
				t.Errorf("trailer: %v", resp.Trailer)
			}
		})

		t.Run("Call"+strings.Title(fn), func(t *testing.T) {
			req := newSignedTestRequest(key, http.MethodPost, path+"?action=call&function="+fn, nil)
			resp, content := doTest(t, handler, req)

			if resp.StatusCode != http.StatusCreated {
				t.Fatal(resp.Status)
			}

			if x := resp.Header.Get(webapi.HeaderLocation); x != webapi.PathModuleRefs+testHashHello {
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

			testStatusResponse(t, resp.Trailer.Get(webapi.HeaderStatus), webapi.Status{
				State: "terminated",
			})

			if len(resp.Trailer) != 1 {
				t.Errorf("trailer: %v", resp.Trailer)
			}
		})

		t.Run("Launch"+strings.Title(fn), func(t *testing.T) {
			req := newSignedTestRequest(key, http.MethodPost, path+"?action=launch&function="+fn, nil)
			resp, content := doTest(t, handler, req)

			if resp.StatusCode != http.StatusCreated {
				t.Fatal(resp.Status)
			}

			if x := resp.Header.Get(webapi.HeaderLocation); x != webapi.PathModuleRefs+testHashHello {
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
		req := newSignedTestRequest(key, http.MethodPost, path+"?action=call&function=test_plugin", nil)
		resp, _ := doTest(t, handler, req)

		if resp.StatusCode != http.StatusCreated {
			t.Fatal(resp.Status)
		}
	})

	t.Run("Ref", func(t *testing.T) {
		req := newSignedTestRequest(key, http.MethodHead, webapi.PathModuleRefs+testHashHello, nil)
		resp, content := doTest(t, handler, req)

		if resp.StatusCode != http.StatusOK {
			t.Fatal(resp.Status)
		}

		if x := resp.Header.Get(webapi.HeaderContentType); x != webapi.ContentTypeWebAssembly {
			t.Error(x)
		}

		if len(content) != 0 {
			t.Error(content)
		}
	})
}

func TestInstance(t *testing.T) {
	ctx := context.Background()
	key := newTestKey()
	handler := newTestHandler(ctx)

	testList := func(t *testing.T, expect interface{}) {
		t.Helper()

		req := newSignedTestRequest(key, http.MethodGet, webapi.PathInstances, nil)
		resp, content := doTest(t, handler, req)

		if resp.StatusCode != http.StatusOK {
			t.Fatal(resp.Status)
		}

		if x := resp.Header.Get(webapi.HeaderContentType); x != contentTypeJSON {
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

	testStatus := func(t *testing.T, instID string, expect webapi.Status) {
		t.Helper()

		req := newSignedTestRequest(key, http.MethodPost, webapi.PathInstances+instID+"?action=status", nil)
		resp, content := doTest(t, handler, req)

		if resp.StatusCode != http.StatusNoContent {
			t.Fatal(resp.Status)
		}

		if _, found := resp.Header[webapi.HeaderContentType]; found {
			t.Fail()
		}

		if len(content) != 0 {
			t.Error(content)
		}

		testStatusResponse(t, resp.Header.Get(webapi.HeaderStatus), expect)
	}

	t.Run("ListEmpty", func(t *testing.T) {
		testList(t, map[string]interface{}{
			"instances": []interface{}{},
		})
	})

	{
		var instID string

		{
			req := newSignedTestRequest(key, http.MethodPost, webapi.PathModuleRefs+testHashHello+"?action=launch&function=main", testProgHello)
			req.Header.Set(webapi.HeaderContentType, webapi.ContentTypeWebAssembly)
			resp, _ := doTest(t, handler, req)

			if resp.StatusCode != http.StatusCreated {
				t.Fatal(resp.Status)
			}

			instID = resp.Header.Get(webapi.HeaderInstance)
		}

		t.Run("StatusRunning", func(t *testing.T) {
			testStatus(t, instID, webapi.Status{
				State: "running",
			})
		})

		t.Run("ListOne", func(t *testing.T) {
			testList(t, map[string]interface{}{
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
			req := newSignedTestRequest(key, http.MethodPost, webapi.PathInstances+instID+"?action=io", nil)
			resp, content := doTest(t, handler, req)

			if resp.StatusCode != http.StatusOK {
				t.Fatal(resp.Status)
			}

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
			req := newSignedTestRequest(key, http.MethodPost, webapi.PathInstances+instID+"?action=wait", nil)
			resp, _ := doTest(t, handler, req)

			if resp.StatusCode != http.StatusNoContent {
				t.Fatal(resp.Status)
			}

			testStatusResponse(t, resp.Header.Get(webapi.HeaderStatus), webapi.Status{
				State: "terminated",
			})
		})

		for i := 0; i < 3; i++ {
			t.Run(fmt.Sprintf("StatusTerminated%d", i), func(t *testing.T) {
				t.Parallel()
				testStatus(t, instID, webapi.Status{
					State: "terminated",
				})
			})
		}
	}

	{
		var instID string

		{
			req := newSignedTestRequest(key, http.MethodPost, webapi.PathModuleRefs+testHashHello+"?action=launch&function=multi", testProgHello)
			req.Header.Set(webapi.HeaderContentType, webapi.ContentTypeWebAssembly)
			resp, _ := doTest(t, handler, req)

			if resp.StatusCode != http.StatusCreated {
				t.Fatal(resp.Status)
			}

			instID = resp.Header.Get(webapi.HeaderInstance)
		}

		t.Run("MultiIO", func(t *testing.T) {
			done := make(chan struct{}, 10)

			for i := 0; i < cap(done); i++ {
				go func() {
					defer func() { done <- struct{}{} }()

					req := newSignedTestRequest(key, http.MethodPost, webapi.PathInstances+instID+"?action=io", nil)
					resp, content := doTest(t, handler, req)

					if resp.StatusCode != http.StatusOK {
						t.Fatal(resp.Status)
					}

					if _, found := resp.Header[webapi.HeaderContentType]; found {
						t.Fail()
					}

					if string(content) != "hello, world\n" {
						t.Errorf("%q", content)
					}

					testStatusResponse(t, resp.Trailer.Get(webapi.HeaderStatus), webapi.Status{
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
			req := newSignedTestRequest(key, http.MethodPost, webapi.PathModuleRefs+testHashSuspend+"?action=launch&function=main", testProgSuspend)
			req.Header.Set(webapi.HeaderContentType, webapi.ContentTypeWebAssembly)
			resp, _ := doTest(t, handler, req)

			if resp.StatusCode != http.StatusCreated {
				t.Fatal(resp.Status)
			}

			instID = resp.Header.Get(webapi.HeaderInstance)
		}

		t.Run("Suspend", func(t *testing.T) {
			req := newSignedTestRequest(key, http.MethodPost, webapi.PathInstances+instID+"?action=suspend", nil)
			resp, _ := doTest(t, handler, req)

			if resp.StatusCode != http.StatusNoContent {
				t.Fatal(resp.Status)
			}

			testStatusResponse(t, resp.Header.Get(webapi.HeaderStatus), webapi.Status{
				State: "suspended",
			})
		})

		t.Run("StatusSuspended", func(t *testing.T) {
			testStatus(t, instID, webapi.Status{
				State: "suspended",
			})
		})

		var snapshotKey string
		var snapshot []byte

		t.Run("Snapshot", func(t *testing.T) {
			req := newSignedTestRequest(key, http.MethodPost, webapi.PathInstances+instID+"?action=snapshot", nil)
			resp, _ := doTest(t, handler, req)

			if resp.StatusCode != http.StatusCreated {
				t.Fatal(resp.Status)
			}

			location := resp.Header.Get(webapi.HeaderLocation)
			if location == "" {
				t.Fatal("no module location")
			}

			snapshotKey = path.Base(location)

			req = newSignedTestRequest(key, http.MethodGet, location, nil)
			resp, snapshot = doTest(t, handler, req)

			if resp.StatusCode != http.StatusOK {
				t.Fatal(resp.Status)
			}

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

			if _, err := wag.Compile(nil, bytes.NewReader(snapshot), dummyResolver{}); err != nil {
				t.Error(err)
			}
		})

		t.Run("ListSuspendedModule", func(t *testing.T) {
			req := newSignedTestRequest(key, http.MethodGet, webapi.PathModuleRefs, nil)
			resp, content := doTest(t, handler, req)

			if resp.StatusCode != http.StatusOK {
				t.Fatal(resp.Status)
			}

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
			req := newSignedTestRequest(key, http.MethodPost, webapi.PathModuleRefs+sha384(snapshot)+"?action=launch", snapshot)
			req.Header.Set(webapi.HeaderContentType, webapi.ContentTypeWebAssembly)
			resp, _ := doTest(t, handler, req)

			if resp.StatusCode != http.StatusCreated {
				t.Fatal(resp.Status)
			}

			restoredID := resp.Header.Get(webapi.HeaderInstance)

			req = newSignedTestRequest(key, http.MethodPost, webapi.PathInstances+restoredID+"?action=suspend", nil)
			resp, _ = doTest(t, handler, req)

			if resp.StatusCode != http.StatusNoContent {
				t.Fatal(resp.Status)
			}
		})
	}
}

// TODO: WebSocket tests

type dummyResolver struct{}

func (dummyResolver) ResolveFunc(module, field string, sig wa.FuncType) (index int, err error) {
	return
}

func (dummyResolver) ResolveGlobal(module, field string, t wa.Type) (value uint64, err error) {
	return
}
