// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gate_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tsavola/gate/server"
	"github.com/tsavola/gate/server/webserver"
	"github.com/tsavola/gate/webapi"
)

func newBenchServer(ctx context.Context) *server.Server {
	config := &server.Config{
		Executor:     newExecutor(ctx, nil).Executor,
		AccessPolicy: server.NewPublicAccess(newServices()),
	}

	return server.New(ctx, config)
}

func newBenchHandler(ctx context.Context) http.Handler {
	config := &webserver.Config{
		Server:    newBenchServer(ctx),
		Authority: "bench",
	}

	return webserver.NewHandler(ctx, "/", config)
}

func BenchmarkCall(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler := newBenchHandler(ctx)
	uri := webapi.PathModuleRefs + hashNop + "?action=call"

	for i := 0; i < b.N; i++ {
		req := newRequest(http.MethodPost, uri, wasmNop)
		req.Header.Set(webapi.HeaderContentType, webapi.ContentTypeWebAssembly)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		resp := w.Result()
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			b.Fatal(resp.Status)
		}
	}
}
