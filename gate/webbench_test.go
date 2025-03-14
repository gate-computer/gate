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

	. "import.name/type/context"
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	executor := newExecutor()
	defer executor.Close()

	benchCall(ctx, b, executor)
}

func BenchmarkCallExecutors(b *testing.B) {
	for n := 2; n <= goruntime.GOMAXPROCS(0)+2; n += 2 {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			benchCallExecutors(b, n)
		})
	}
}

func benchCallExecutors(b *testing.B, count int) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var executors []runtime.ProcessFactory

	for i := 0; i < count; i++ {
		e := newExecutor()
		defer e.Close()

		executors = append(executors, e)
	}

	benchCall(ctx, b, runtime.DistributeProcesses(executors...))
}

func benchCall(ctx Context, b *testing.B, factory runtime.ProcessFactory) {
	server, err := newBenchServer(factory)
	if err != nil {
		b.Fatal(err)
	}
	defer server.Shutdown(ctx)

	handler := newBenchHandler(server)
	uri := web.PathKnownModules + hashNop + "?action=call"

	procs := goruntime.GOMAXPROCS(0)
	loops := (b.N + procs - 1) / procs

	done := make(chan int, procs)

	for i := 0; i < procs; i++ {
		go func() {
			var status int
			defer func() {
				done <- status
			}()

			for i := 0; i < loops; i++ {
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

	for i := 0; i < procs; i++ {
		if status := <-done; status != http.StatusOK {
			b.Error(status)
		}
	}
}
