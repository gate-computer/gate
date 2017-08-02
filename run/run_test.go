package run_test

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path"
	"testing"

	"github.com/tsavola/gate/internal/runtest"
	"github.com/tsavola/gate/run"
	"github.com/tsavola/wag"
	"github.com/tsavola/wag/dewag"
	"github.com/tsavola/wag/wasm"
)

type readWriter struct {
	io.Reader
	io.Writer
}

func openProgram(testName string) (f *os.File) {
	f, err := os.Open(path.Join(os.Getenv("GATE_TEST_DIR"), testName, "prog.wasm"))
	if err != nil {
		panic(err)
	}
	return
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

	env := runtest.NewEnvironment()
	defer env.Close()

	wasm := openProgram(testName)
	defer wasm.Close()

	m := wag.Module{
		MainSymbol: "main",
	}

	err := m.Load(bufio.NewReader(wasm), env, new(bytes.Buffer), nil, run.RODataAddr, nil)
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
			panic(err)
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
