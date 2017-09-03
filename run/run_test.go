// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package run_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path"
	"testing"

	"github.com/tsavola/gate/internal/runtest"
	"github.com/tsavola/gate/packet"
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

	rt := runtest.NewRuntime()
	defer rt.Close()

	wasm := openProgram(testName)
	defer wasm.Close()

	var m wag.Module

	err := run.Load(&m, bufio.NewReader(wasm), rt.Runtime, new(bytes.Buffer), nil, nil)
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

	var (
		image run.Image
		proc  run.Process
	)
	defer image.Close()
	defer proc.Close()

	err = image.Init()
	if err != nil {
		t.Fatalf("image error: %v", err)
	}

	err = proc.Init(context.Background(), rt.Runtime, &image, os.Stdout)
	if err != nil {
		return
	}

	err = image.Populate(&m, memorySize, stackSize)
	if err != nil {
		t.Fatalf("image error: %v", err)
	}

	exit, trap, err := run.Run(context.Background(), rt.Runtime, &proc, &image, &testServiceRegistry{origin: &output})
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

		if err := image.DumpGlobalsMemoryStack(f); err != nil {
			t.Fatalf("dump error: %v", err)
		}
	}

	return
}

type testServiceRegistry struct {
	origin io.Writer
}

func (services *testServiceRegistry) Info(name string) (info run.ServiceInfo) {
	var code uint16

	switch name {
	case "origin":
		code = 1

	case "test1":
		code = 2
		info.Version = 1337

	case "test2":
		code = 3
		info.Version = 12765
	}

	binary.LittleEndian.PutUint16(info.Code[:], code)
	return
}

func (services *testServiceRegistry) Serve(ctx context.Context, ops <-chan packet.Buf, evs chan<- packet.Buf, maxContentSize int) (err error) {
	defer close(evs)

	for op := range ops {
		switch op.Code().Int() {
		case 1:
			if _, err := services.origin.Write(op.Content()); err != nil {
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
