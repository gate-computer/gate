// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gate_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	goruntime "runtime"
	"strconv"
	"testing"

	"gate.computer/gate/runtime"
	"gate.computer/gate/server"
	"gate.computer/gate/server/webserver"
	"gate.computer/gate/web"
	"github.com/stretchr/testify/assert"

	. "import.name/testing/mustr"
)

func newBenchServer(factory runtime.ProcessFactory) (*server.Server, error) {
	config := &server.Config{
		ProcessFactory: factory,
		AccessPolicy:   server.NewPublicAccess(newServices()),
	}

	return server.New(context.Background(), config)
}

func newBenchHandler(s *server.Server) http.Handler {
	config := &webserver.Config{
		Server:    s,
		Authority: "bench",
		Origins:   []string{"null"},
	}

	return webserver.NewHandler("/", config)
}

func BenchmarkCall(b *testing.B) {
	executor := newExecutor()
	defer executor.Close()

	benchCall(b, executor)
}

func BenchmarkCallExecutors(b *testing.B) {
	for n := 2; n <= goruntime.GOMAXPROCS(0)+2; n += 2 {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			benchCallExecutors(b, n)
		})
	}
}

func benchCallExecutors(b *testing.B, count int) {
	var executors []runtime.ProcessFactory

	for range count {
		e := newExecutor()
		defer e.Close()

		executors = append(executors, e)
	}

	benchCall(b, runtime.DistributeProcesses(executors...))
}

func benchCall(b *testing.B, factory runtime.ProcessFactory) {
	server := Must(b, R(newBenchServer(factory)))
	defer server.Shutdown(b.Context())

	handler := newBenchHandler(server)
	uri := web.PathKnownModules + hashNop + "?action=call"

	procs := goruntime.GOMAXPROCS(0)
	loops := (b.N + procs - 1) / procs

	done := make(chan int, procs)

	for range procs {
		go func() {
			var status int
			defer func() {
				done <- status
			}()

			for range loops {
				req := newRequest(http.MethodPost, uri, wasmNop)
				req.Header.Set(web.HeaderContentType, web.ContentTypeWebAssembly)
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, req)
				resp := w.Result()
				resp.Body.Close()

				status = resp.StatusCode
				if status != http.StatusOK {
					return
				}
			}
		}()
	}

	for range procs {
		assert.Equal(b, <-done, http.StatusOK)
	}
}
