package run_test

import (
	"bufio"
	"encoding/binary"
	"os"
	"testing"

	"github.com/tsavola/wag"
	"github.com/tsavola/wag/dewag"
	"github.com/tsavola/wag/wasm"

	"."
)

const (
	dumpText = true
)

func TestRun(t *testing.T) {
	const (
		memorySizeLimit = wasm.Page
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

	err = m.Load(r, env, nil, nil, run.RODataAddr, nil)
	if err != nil {
		t.Fatalf("load error: %v", err)
	}

	if dumpText && testing.Verbose() {
		dewag.PrintTo(os.Stdout, m.Text(), m.FunctionMap())
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

	output, err := run.Run(env, payload)
	dumpOutput(t, output)
	if err != nil {
		t.Fatalf("run error: %v", err)
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

func dumpOutput(t *testing.T, data []byte) {
	for len(data) > 0 {
		if len(data) >= 8 {
			size := binary.LittleEndian.Uint32(data)
			if size >= 8 && size <= uint32(len(data)) {
				t.Logf("op size:    %d\n", size)
				t.Logf("op code:    %d\n", binary.LittleEndian.Uint16(data[4:]))
				t.Logf("op flags:   0x%x\n", binary.LittleEndian.Uint16(data[6:]))
				t.Logf("op payload: %#v\n", string(data[8:size]))
				data = data[size:]
				continue
			}
		}
		t.Logf("garbage: %#v\n", string(data))
		break
	}
}
