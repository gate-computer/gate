package run_test

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/tsavola/wag"
	"github.com/tsavola/wag/dewag"
	"github.com/tsavola/wag/wasm"

	"."
)

type readWriter struct {
	io.Reader
	io.Writer
}

const (
	dumpText = true
)

func TestRun(t *testing.T) {
	const (
		memorySizeLimit = 24 * wasm.Page
		stackSize       = 4096
	)

	executorBin := os.Getenv("GATE_TEST_EXECUTOR")
	loaderBin := os.Getenv("GATE_TEST_LOADER")
	wasmPath := os.Getenv("GATE_TEST_WASM")

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

	var m wag.Module

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

	var output bytes.Buffer

	exit, trap, err := run.Run(env, payload, readWriter{new(bytes.Buffer), &output}, os.Stdout)
	t.Logf("output: %#v\n", string(output.Bytes()))
	if err != nil {
		t.Fatalf("run error: %v", err)
	} else if trap != 0 {
		t.Fatalf("run trap: %s", trap)
	} else if exit != 0 {
		t.Fatalf("run exit: %s", exit)
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
}
