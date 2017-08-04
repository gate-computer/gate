package server

import (
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"io"
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
	api "github.com/tsavola/gate/server/serverapi"
	"github.com/tsavola/gate/service/origin"
	"github.com/tsavola/wag/wasm"
)

var handler = NewHandler(context.Background(), "/", NewState(Settings{
	MemorySizeLimit: 64 * wasm.Page,
	StackSize:       65536,
	Env:             runtest.NewEnvironment(),
	Services:        func(r io.Reader, w io.Writer) run.ServiceRegistry { return origin.CloneRegistryWith(nil, r, w) },
	Log:             log.New(os.Stderr, "log: ", 0),
	Debug:           os.Stdout,
}))

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

func do(t *testing.T, req *http.Request) (resp *http.Response, body []byte) {
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	resp = w.Result()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return
}

func doJSON(t *testing.T, req *http.Request) (resp *http.Response, content map[string]map[string]interface{}) {
	resp, body := do(t, req)
	if strings.Split(resp.Header.Get("Content-Type"), ";")[0] == "application/json" {
		if err := json.Unmarshal(body, &content); err != nil {
			t.Fatal(err)
		}
	}
	return
}

func TestLoadNotFound(t *testing.T) {
	req := httptest.NewRequest("POST", "/load", bytes.NewBufferString(`{"program": {"id": "535561601a3a8550", "sha512": "c4d28256984e0fc6cc645ee184a49fbd9efc07d29a07bf43baab07deeac21f255a3796c6d04132a86a141847d118a3e0bf729681ad7910ae2d8e99fec1327430"}}`))
	req.Header.Set("Content-Type", "application/json")

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
		req.Header.Set("X-Gate-Program-Sha512", progHash)

		resp, content := doJSON(t, req)
		if resp.StatusCode != http.StatusOK {
			t.Fatal(resp.StatusCode)
		}

		if len(content) != 1 || len(content["program"]) != 2 {
			t.Fatal(content)
		}

		progId = content["program"]["id"].(string)
		if progId == "" {
			t.Fatal("no program id")
		}

		if content["program"]["sha512"].(string) != progHash {
			t.Fatal("bad program hash")
		}
	}

	{
		req := httptest.NewRequest("POST", "/load", bytes.NewBufferString(`{"program": {"id": "`+progId+`", "sha512": "c4d28256984e0fc6cc645ee184a49fbd9efc07d29a07bf43baab07deeac21f255a3796c6d04132a86a141847d118a3e0bf729681ad7910ae2d8e99fec1327430"}}`))
		req.Header.Set("Content-Type", "application/json")

		resp, _ := do(t, req)
		if resp.StatusCode != http.StatusForbidden {
			t.Fatal(resp.StatusCode)
		}
	}

	progJSON := `{"program": {"id": "` + progId + `", "sha512": "` + progHash + `"}}`

	{
		req := httptest.NewRequest("POST", "/load", bytes.NewBufferString(progJSON))
		req.Header.Set("Content-Type", "application/json")

		resp, content := doJSON(t, req)
		if resp.StatusCode != http.StatusOK {
			t.Fatal(resp.StatusCode)
		}

		if len(content) != 0 {
			t.Fatal(content)
		}
	}

	var instId string

	{
		req := httptest.NewRequest("POST", "/spawn", bytes.NewBufferString(progJSON))
		req.Header.Set("Content-Type", "application/json")

		resp, content := doJSON(t, req)
		if resp.StatusCode != http.StatusOK {
			t.Fatal(resp.StatusCode)
		}

		if len(content) != 1 || len(content["instance"]) != 1 {
			t.Fatal(content)
		}

		instId = content["instance"]["id"].(string)
		if instId == "" {
			t.Fatal("no instance id")
		}
	}

	{
		req := httptest.NewRequest("POST", "/communicate", bytes.NewBuffer(nil))
		req.Header.Set("X-Gate-Instance-Id", instId)

		resp, body := do(t, req)
		if resp.StatusCode != http.StatusOK {
			t.Fatal(resp.StatusCode)
		}

		if string(body) != "hello world\n" {
			t.Error(body)
		}
	}

	{
		req := httptest.NewRequest("POST", "/communicate", bytes.NewBuffer(nil))
		req.Header.Set("X-Gate-Instance-Id", instId)

		resp, _ := do(t, req)
		if resp.StatusCode != http.StatusConflict {
			t.Fatal(resp.StatusCode)
		}
	}

	instJSON := `{"instance": {"id": "` + instId + `"}}`

	{
		req := httptest.NewRequest("POST", "/wait", bytes.NewBufferString(instJSON))
		req.Header.Set("Content-Type", "application/json")

		resp, content := doJSON(t, req)
		if resp.StatusCode != http.StatusOK {
			t.Fatal(resp.StatusCode)
		}

		if content["result"]["exit"].(float64) != 0 {
			t.Error(content)
		}
	}

	{
		req := httptest.NewRequest("POST", "/wait", bytes.NewBufferString(instJSON))
		req.Header.Set("Content-Type", "application/json")

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

	if err := conn.WriteMessage(websocket.BinaryMessage, progData); err != nil {
		panic(err)
	}

	var running api.Running
	if err := conn.ReadJSON(&running); err != nil {
		t.Fatal(err)
	}
	if running.Program.Id == "" || running.Program.SHA512 == "" || running.Instance.Id == "" {
		t.Fatalf("%v", running)
	}

	frameType, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if frameType != websocket.BinaryMessage || string(data) != "hello world\n" {
		t.Fatal(data)
	}

	var finished api.Finished
	if err := conn.ReadJSON(&finished); err != nil {
		t.Fatal(err)
	}
	if finished.Result.Exit != 0 || finished.Result.Trap != "" || finished.Result.Error != "" {
		t.Fatalf("%v", finished)
	}
}
