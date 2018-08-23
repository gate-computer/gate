// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package run_test

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os"
	"path"
	"sync"
	"testing"

	"github.com/tsavola/gate/internal/runtest"
	"github.com/tsavola/gate/packet"
	"github.com/tsavola/gate/run"
	"github.com/tsavola/wag/compile"
	"github.com/tsavola/wag/disasm"
	"github.com/tsavola/wag/insnmap"
	"github.com/tsavola/wag/section"
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

func TestRust(t *testing.T) {
	output := testRun(t, "rust")
	t.Log(string(output.Bytes()))
}

func TestServices(t *testing.T) {
	testRun(t, "services")
}

func testRun(t *testing.T, testName string) (output bytes.Buffer) {
	const (
		memorySizeLimit = 24 * wasm.Page
		stackSize       = 24 * 4096
	)

	rt := runtest.NewRuntime(nil)
	defer rt.Close()

	wasm := openProgram(testName)
	defer wasm.Close()

	var (
		nameSection section.NameSection
		insnMap     insnmap.Map
	)

	m := compile.Module{
		UnknownSectionLoader: section.UnknownLoaders{"name": nameSection.Load}.Load,
	}

	err := run.Load(&m, bufio.NewReader(wasm), rt.Runtime, nil, nil, &insnMap)
	if err != nil {
		t.Fatalf("load error: %v", err)
	}

	if dumpText && testing.Verbose() {
		disasm.Fprint(os.Stdout, m.Text(), insnMap.FuncAddrs, nil)
	}

	_, memorySize := m.MemoryLimits()
	if memorySize > memorySizeLimit {
		memorySize = memorySizeLimit
	}

	var (
		image run.Image
		proc  run.Process
	)
	defer image.Release(rt.Runtime)
	defer proc.Kill(rt.Runtime)

	err = image.Init(context.Background(), rt.Runtime)
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

	if false {
		var buf bytes.Buffer

		if err := disasm.Fprint(&buf, m.Text(), insnMap.FuncAddrs, &nameSection); err == nil {
			t.Logf("disassembly:\n%s", string(buf.Bytes()))
		} else {
			t.Errorf("disassembly error: %v", err)
		}
	}

	stacktrace := true

	exit, trapId, err := run.Run(context.Background(), rt.Runtime, &proc, &image, &testServiceRegistry{origin: &output})
	if err != nil {
		t.Errorf("run error: %v", err)
	} else if trapId != 0 {
		t.Errorf("run trap: %s", trapId)
	} else if exit != 0 {
		t.Errorf("run exit: %d", exit)
	} else {
		stacktrace = false
	}

	if stacktrace {
		var buf bytes.Buffer

		if err := image.DumpStacktrace(&buf, &m, &insnMap.Map, &nameSection); err == nil {
			t.Logf("stacktrace:\n%s", string(buf.Bytes()))
		} else {
			t.Errorf("stacktrace error: %v", err)
		}
	}

	if name := os.Getenv("GATE_TEST_DUMP"); name != "" {
		f, err := os.Create(name)
		if err != nil {
			panic(err)
		}
		defer f.Close()

		if err := image.DumpGlobalsMemoryStack(f); err != nil {
			t.Errorf("dump error: %v", err)
		}
	}

	return
}

type testServiceRegistry struct {
	origin io.Writer
}

func (services *testServiceRegistry) StartServing(ctx context.Context, ops <-chan packet.Buf, evs chan<- packet.Buf, maxContentSize int,
) run.ServiceDiscoverer {
	d := new(testServiceDiscoverer)

	go func() {
		defer close(evs)

		for op := range ops {
			i := op.Code().Int16()

			d.nameLock.Lock()
			name := d.names[i]
			d.nameLock.Unlock()

			switch name {
			case "origin":
				if _, err := services.origin.Write(op.Content()); err != nil {
					panic(err)
				}
			}
		}
	}()

	return d
}

type testServiceDiscoverer struct {
	services []run.Service
	nameLock sync.Mutex
	names    []string
}

func (d *testServiceDiscoverer) Discover(names []string) []run.Service {
	for _, name := range names {
		var s run.Service

		switch name {
		case "origin":
			s.SetAvailable(0)

		case "test1":
			s.SetAvailable(1337)

		case "test2":
			s.SetAvailable(12765)
		}

		d.services = append(d.services, s)

		d.nameLock.Lock()
		d.names = append(d.names, name)
		d.nameLock.Unlock()
	}

	return d.services
}

func (d *testServiceDiscoverer) NumServices() int {
	return len(d.services)
}
