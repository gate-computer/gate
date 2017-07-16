package run_test

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"os/user"
	"path"
	"strconv"
	"testing"

	"github.com/tsavola/wag"
	"github.com/tsavola/wag/dewag"
	"github.com/tsavola/wag/wasm"

	"github.com/tsavola/gate/run"
)

func parseId(t *testing.T, s string) uint {
	n, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		t.Fatal(err)
	}
	return uint(n)
}

type readWriter struct {
	io.Reader
	io.Writer
}

const (
	dumpText = false
)

func TestAlloc(t *testing.T) {
	output := testRun(t, "alloc")
	t.Log(string(output.Bytes()))
}

func TestHello(t *testing.T) {
	output := testRun(t, "hello")
	if s := string(output.Bytes()); s != "hello world\n" {
		t.Fatalf("output: %#v", s)
	}
}

func TestServices(t *testing.T) {
	testRun(t, "services")
}

func testRun(t *testing.T, testName string) (output bytes.Buffer) {
	const (
		memorySizeLimit = 24 * wasm.Page
		stackSize       = 4096
	)

	bootUser, err := user.Lookup(os.Getenv("GATE_TEST_BOOTUSER"))
	if err != nil {
		t.Fatal(err)
	}

	execUser, err := user.Lookup(os.Getenv("GATE_TEST_EXECUSER"))
	if err != nil {
		t.Fatal(err)
	}

	pipeGroup, err := user.LookupGroup(os.Getenv("GATE_TEST_PIPEGROUP"))
	if err != nil {
		t.Fatal(err)
	}

	config := run.Config{
		LibDir: os.Getenv("GATE_TEST_LIBDIR"),
		Uids: [2]uint{
			parseId(t, bootUser.Uid),
			parseId(t, execUser.Uid),
		},
		Gids: [3]uint{
			parseId(t, bootUser.Gid),
			parseId(t, execUser.Gid),
			parseId(t, pipeGroup.Gid),
		},
	}

	wasmPath := path.Join(os.Getenv("GATE_TEST_DIR"), testName, "prog.wasm")

	env, err := run.NewEnvironment(&config)
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close()

	f, err := os.Open(wasmPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	r := bufio.NewReader(f)

	m := wag.Module{
		MainSymbol: "main",
	}

	err = m.Load(r, env, new(bytes.Buffer), nil, run.RODataAddr, nil)
	if err != nil {
		t.Fatalf("load error: %v", err)
	}

	if dumpText && testing.Verbose() {
		dewag.PrintTo(os.Stdout, m.Text(), m.FunctionMap(), nil)
	}

	_, memorySize := m.MemoryLimits()
	if memorySize > memorySizeLimit {
		memorySize = memorySizeLimit
	}

	payload, err := run.NewPayload(&m, memorySize, stackSize)
	if err != nil {
		t.Fatalf("payload error: %v", err)
	}
	defer payload.Close()

	exit, trap, err := run.Run(env, payload, &testServiceRegistry{origin: &output}, os.Stdout)
	if err != nil {
		t.Fatalf("run error: %v", err)
	} else if trap != 0 {
		t.Fatalf("run trap: %s", trap)
	} else if exit != 0 {
		t.Fatalf("run exit: %d", exit)
	}

	if name := os.Getenv("GATE_TEST_DUMP"); name != "" {
		f, err := os.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()

		if err := payload.DumpGlobalsMemoryStack(f); err != nil {
			t.Fatalf("dump error: %v", err)
		}
	}

	return
}

type testServiceRegistry struct {
	origin io.Writer
}

func (services *testServiceRegistry) Info(name string) (info run.ServiceInfo) {
	switch name {
	case "origin":
		info.Code = 1
		info.Version = 0

	case "test1":
		info.Code = 2
		info.Version = 1337

	case "test2":
		info.Code = 3
		info.Version = 12765
	}

	return
}

func (services *testServiceRegistry) Serve(ops <-chan []byte, evs chan<- []byte) (err error) {
	defer close(evs)

	for op := range ops {
		switch binary.LittleEndian.Uint16(op[6:]) {
		case 1:
			if _, err := services.origin.Write(op[8:]); err != nil {
				panic(err)
			}

		case 2, 3:
			// ok

		default:
			err = errors.New("invalid service code")
			return
		}
	}

	return
}
