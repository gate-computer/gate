// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webserver

import (
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/hex"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/tsavola/gate/internal/runtest"
	"github.com/tsavola/gate/run"
	"github.com/tsavola/gate/server"
	"github.com/tsavola/gate/server/serverconfig"
	"github.com/tsavola/gate/service"
	"github.com/tsavola/gate/service/origin"
	api "github.com/tsavola/gate/webapi"
)

func services(s *serverconfig.Server) run.ServiceRegistry {
	r := service.Defaults.Clone()
	origin.New(s.Origin.R, s.Origin.W).Register(r)
	return r
}

var handler = NewHandler(context.Background(), "/", server.NewState(context.Background(), &serverconfig.Config{
	Env:      runtest.NewEnvironment().Environment,
	Services: services,
	Log:      log.New(os.Stderr, "log: ", 0),
	Debug:    os.Stdout,
}), nil)

var (
	progData []byte
	progHash string
)

func init() {
	var err error

	progData, err = ioutil.ReadFile(path.Join(os.Getenv("GATE_TEST_DIR"), "hello", "prog.wasm"))
	if err != nil {
		panic(err)
	}

	hash := sha512.New()
	hash.Write(progData)
	progHash = hex.EncodeToString(hash.Sum(nil))
}

func do(t *testing.T, req *http.Request) (resp *http.Response, content []byte) {
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	resp = w.Result()
	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return
}

func TestLoadNotFound(t *testing.T) {
	req := httptest.NewRequest("POST", "/load", bytes.NewBuffer(nil))
	req.Header.Set(api.HeaderProgramId, "535561601a3a8550")
	req.Header.Set(api.HeaderProgramSHA512, "c4d28256984e0fc6cc645ee184a49fbd9efc07d29a07bf43baab07deeac21f255a3796c6d04132a86a141847d118a3e0bf729681ad7910ae2d8e99fec1327430")

	resp, _ := do(t, req)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatal(resp.StatusCode)
	}
}

func TestLoadSpawnCommunicateWait(t *testing.T) {
	var progId string

	{
		req := httptest.NewRequest("POST", "/load", bytes.NewBuffer(progData))
		req.Header.Set("Content-Type", "application/wasm")
		req.Header.Set(api.HeaderProgramSHA512, progHash)

		resp, _ := do(t, req)
		if resp.StatusCode != http.StatusOK {
			t.Fatal(resp.StatusCode)
		}

		progId = resp.Header.Get(api.HeaderProgramId)
		if progId == "" {
			t.Fatal("no program id")
		}
	}

	{
		req := httptest.NewRequest("POST", "/load", bytes.NewBuffer(nil))
		req.Header.Set(api.HeaderProgramId, progId)
		req.Header.Set(api.HeaderProgramSHA512, "c4d28256984e0fc6cc645ee184a49fbd9efc07d29a07bf43baab07deeac21f255a3796c6d04132a86a141847d118a3e0bf729681ad7910ae2d8e99fec1327430")

		resp, _ := do(t, req)
		if resp.StatusCode != http.StatusForbidden {
			t.Fatal(resp.StatusCode)
		}
	}

	{
		req := httptest.NewRequest("POST", "/load", bytes.NewBuffer(nil))
		req.Header.Set(api.HeaderProgramId, progId)
		req.Header.Set(api.HeaderProgramSHA512, progHash)

		resp, _ := do(t, req)
		if resp.StatusCode != http.StatusOK {
			t.Fatal(resp.StatusCode)
		}
	}

	var instId string

	{
		req := httptest.NewRequest("POST", "/spawn", bytes.NewBuffer(nil))
		req.Header.Set(api.HeaderProgramId, progId)
		req.Header.Set(api.HeaderProgramSHA512, progHash)

		resp, _ := do(t, req)
		if resp.StatusCode != http.StatusOK {
			t.Fatal(resp.StatusCode)
		}

		instId = resp.Header.Get(api.HeaderInstanceId)
		if instId == "" {
			t.Fatal("no instance id")
		}
	}

	{
		req := httptest.NewRequest("POST", "/communicate", bytes.NewBufferString(""))
		req.Header.Set(api.HeaderInstanceId, instId)

		resp, content := do(t, req)
		if resp.StatusCode != http.StatusOK {
			t.Fatal(resp.StatusCode)
		}

		if string(content) != "hello world\n" {
			t.Error(content)
		}
	}

	{
		req := httptest.NewRequest("POST", "/communicate", bytes.NewBufferString("garbage"))
		req.Header.Set(api.HeaderInstanceId, instId)

		resp, _ := do(t, req)
		if resp.StatusCode != http.StatusConflict {
			t.Fatal(resp.StatusCode)
		}
	}

	{
		req := httptest.NewRequest("POST", "/wait", bytes.NewBuffer(nil))
		req.Header.Set(api.HeaderInstanceId, instId)

		resp, _ := do(t, req)
		if resp.StatusCode != http.StatusOK {
			t.Fatal(resp.StatusCode)
		}

		if resp.Header.Get(api.HeaderExitStatus) != "0" {
			t.Errorf("%#v", resp.Header)
		}
	}

	{
		req := httptest.NewRequest("POST", "/wait", bytes.NewBuffer(nil))
		req.Header.Set(api.HeaderInstanceId, instId)

		resp, _ := do(t, req)
		if resp.StatusCode != http.StatusNotFound {
			t.Fatal(resp.StatusCode)
		}
	}
}

func TestRun(t *testing.T) {
	server := httptest.NewServer(handler)
	defer server.Close()

	var d websocket.Dialer
	conn, _, err := d.Dial(strings.Replace(server.URL, "http", "ws", 1)+"/run", nil)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	err = conn.WriteJSON(api.Run{
		ProgramSHA512: progHash,
	})
	if err != nil {
		panic(err)
	}

	if err := conn.WriteMessage(websocket.BinaryMessage, progData); err != nil {
		panic(err)
	}

	var running api.Running
	if err := conn.ReadJSON(&running); err != nil {
		t.Fatal(err)
	}
	if running.InstanceId == "" {
		t.Fatal("no instance id")
	}
	if running.ProgramId == "" {
		t.Fatal("no program id")
	}

	frameType, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if frameType != websocket.BinaryMessage || string(data) != "hello world\n" {
		t.Fatal(data)
	}

	var result api.Result
	if err := conn.ReadJSON(&result); err != nil {
		t.Fatal(err)
	}
	if result.ExitStatus == nil || *result.ExitStatus != 0 || result.TrapId != 0 {
		t.Fatalf("result: %#v", result)
	}
}
