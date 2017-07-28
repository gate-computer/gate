package server

import (
	"bytes"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/user"
	"path"
	"strconv"
	"strings"
	"testing"

	"github.com/tsavola/wag/wasm"

	"github.com/tsavola/gate/run"
	"github.com/tsavola/gate/service"
	"github.com/tsavola/gate/service/origin"
)

func parseId(s string) uint {
	n, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		panic(err)
	}
	return uint(n)
}

func newEnvironment() *run.Environment {
	bootUser, err := user.Lookup(os.Getenv("GATE_TEST_BOOTUSER"))
	if err != nil {
		panic(err)
	}

	execUser, err := user.Lookup(os.Getenv("GATE_TEST_EXECUSER"))
	if err != nil {
		panic(err)
	}

	pipeGroup, err := user.LookupGroup(os.Getenv("GATE_TEST_PIPEGROUP"))
	if err != nil {
		panic(err)
	}

	config := run.Config{
		LibDir: os.Getenv("GATE_TEST_LIBDIR"),
		Uids: [2]uint{
			parseId(bootUser.Uid),
			parseId(execUser.Uid),
		},
		Gids: [3]uint{
			parseId(bootUser.Gid),
			parseId(execUser.Gid),
			parseId(pipeGroup.Gid),
		},
	}

	env, err := run.NewEnvironment(&config)
	if err != nil {
		panic(err)
	}

	return env
}

var handler = NewHandler("/", NewState(Settings{
	MemorySizeLimit: 64 * wasm.Page,
	StackSize:       65536,
	Env:             newEnvironment(),
	Services: func(r io.Reader, w io.Writer) run.ServiceRegistry {
		return origin.CloneRegistryWith(service.Defaults, r, w)
	},
	Log:   log.New(os.Stderr, "log: ", 0),
	Debug: os.Stdout,
}))

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

func TestLoadRunOriginWait(t *testing.T) {
	prog, err := ioutil.ReadFile(path.Join(os.Getenv("GATE_TEST_DIR"), "hello", "prog.wasm"))
	if err != nil {
		panic(err)
	}

	hash := sha512.New()
	hash.Write(prog)
	progHash := hex.EncodeToString(hash.Sum(nil))

	var progId string

	{
		req := httptest.NewRequest("POST", "/load", bytes.NewBuffer(prog))
		req.Header.Set("Content-Type", "application/wasm")

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
		req := httptest.NewRequest("POST", "/run", bytes.NewBufferString(progJSON))
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
		req := httptest.NewRequest("POST", "/origin/"+instId, bytes.NewBuffer(nil))

		resp, body := do(t, req)
		if resp.StatusCode != http.StatusOK {
			t.Fatal(resp.StatusCode)
		}

		if string(body) != "hello world\n" {
			t.Error(body)
		}
	}

	{
		req := httptest.NewRequest("POST", "/origin/"+instId, bytes.NewBuffer(nil))

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
