package run_test

import (
	"bufio"
	"io/ioutil"
	"os"
	"os/exec"
	"testing"

	"github.com/tsavola/wag"
	"github.com/tsavola/wag/wasm"

	"."
)

const (
	dumpText = true
)

func TestRun(t *testing.T) {
	const (
		memorySizeLimit = 4096 * wasm.Page
		stackSize       = 8 * 1024 * 1024
	)

	executorBin := os.Getenv("GATE_TEST_EXECUTOR")
	loaderBin := os.Getenv("GATE_TEST_LOADER")
	wasmPath := os.Getenv("GATE_TEST_WASM")

	env, err := run.NewEnvironment(executorBin, loaderBin)
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

	objdump(m.Text())

	_, memorySize := m.MemoryLimits()
	if memorySize > memorySizeLimit {
		memorySize = memorySizeLimit
	}

	payload, err := run.NewPayload(&m, memorySize, stackSize)
	if err != nil {
		t.Fatalf("payload error: %v", err)
	}

	err = run.Run(env, payload)
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
}

func objdump(text []byte) {
	if dumpText {
		f, err := ioutil.TempFile("", "")
		if err != nil {
			panic(err)
		}
		_, err = f.Write(text)
		f.Close()
		defer os.Remove(f.Name())
		if err != nil {
			panic(err)
		}

		cmd := exec.Command("objdump", "-D", "-bbinary", "-mi386:x86-64", f.Name())
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			panic(err)
		}
	}
}
