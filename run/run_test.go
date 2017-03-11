package run_test

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"path"
	"testing"

	"github.com/tsavola/wag"
	"github.com/tsavola/wag/dewag"
	"github.com/tsavola/wag/wasm"

	"github.com/tsavola/gate/run"
)

type readWriter struct {
	io.Reader
	io.Writer
}

const (
	dumpText = false
)

func TestAlloc(t *testing.T) {
	testRun(t, "alloc")
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

	executorBin := os.Getenv("GATE_TEST_EXECUTOR")
	loaderBin := os.Getenv("GATE_TEST_LOADER")
	wasmPath := path.Join(os.Getenv("GATE_TEST_DIR"), testName, "prog.wasm")

	env, err := run.NewEnvironment(executorBin, loaderBin, loaderBin+".symbols")
	if err != nil {
		t.Fatal(err)
	}

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

	exit, trap, err := run.Run(env, payload, readWriter{new(bytes.Buffer), &output}, services{}, os.Stdout)
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

type services struct{}

func (services) Info(name string) run.ServiceInfo {
	var (
		atom    uint32
		version uint32
	)

	switch name {
	case "test1":
		atom = 1
		version = 1337

	case "test2":
		atom = 2
		version = 12765
	}

	return run.MakeServiceInfo(atom, version)
}

func (services) Message(payload []byte, atom uint32) (found bool) {
	switch atom {
	case 1, 2:
		found = true
	}

	return
}
