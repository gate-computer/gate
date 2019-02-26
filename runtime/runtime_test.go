// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/tsavola/gate/entry"
	"github.com/tsavola/gate/image"
	"github.com/tsavola/gate/internal/test/runtimeutil"
	"github.com/tsavola/gate/packet"
	"github.com/tsavola/gate/runtime"
	"github.com/tsavola/gate/runtime/abi"
	"github.com/tsavola/wag/binding"
	"github.com/tsavola/wag/compile"
	"github.com/tsavola/wag/object"
	"github.com/tsavola/wag/object/debug"
	"github.com/tsavola/wag/object/stack/stacktrace"
	"github.com/tsavola/wag/trap"
	"github.com/tsavola/wag/wa"
)

const (
	testMaxTextSize   = 32 * 1024 * 1024
	testMaxMemorySize = 128 * 1024 * 1024
	testStackSize     = wa.PageSize
)

var (
	testProgNop        = runtimeutil.MustReadFile("../testdata/nop.wasm")
	testProgHello      = runtimeutil.MustReadFile("../testdata/hello.wasm")
	testProgHelloDebug = runtimeutil.MustReadFile("../testdata/hello-debug.wasm")
	testProgSuspend    = runtimeutil.MustReadFile("../testdata/suspend.wasm")
)

func prepareTest(exec *runtimeutil.Executor, arcBack image.LocalStorage, exeBack image.BackingStore, r compile.Reader, arcModuleSize int, codeMap *object.CallMap,
) (mod compile.Module, ref image.ExecutableRef, build *image.Build) {
	mod, err := compile.LoadInitialSections(nil, r)
	if err != nil {
		panic(err)
	}

	if err := binding.BindImports(&mod, abi.Imports); err != nil {
		panic(err)
	}

	ref, err = image.NewExecutableRef(exeBack)
	if err != nil {
		panic(err)
	}

	build, err = image.NewBuild(arcBack, exeBack, ref, arcModuleSize, testMaxTextSize, codeMap)
	if err != nil {
		panic(err)
	}

	return
}

func buildTest(exec *runtimeutil.Executor, arcKey string, arcBack image.LocalStorage, exeBack image.BackingStore, objectMapper compile.ObjectMapper, codeMap *object.CallMap, r compile.Reader, moduleSize int, function string,
) (arc image.LocalArchive, exe *image.Executable, mod compile.Module) {
	mod, ref, build := prepareTest(exec, arcBack, exeBack, r, moduleSize, codeMap)
	defer build.Close()
	defer ref.Close()

	var codeConfig = &compile.CodeConfig{
		Text:   build.TextBuffer(),
		Mapper: objectMapper,
	}

	if err := compile.LoadCodeSection(codeConfig, r, mod); err != nil {
		panic(err)
	}

	// dump.Text(os.Stderr, codeConfig.Text.Bytes(), 0, codeMap.FuncAddrs, nil)

	maxMemorySize := mod.MemorySizeLimit()
	if maxMemorySize > testMaxMemorySize {
		maxMemorySize = testMaxMemorySize
	}

	if err := build.FinishText(0, testStackSize, mod.GlobalsSize(), mod.InitialMemorySize(), maxMemorySize); err != nil {
		panic(err)
	}

	var entryIndex uint32
	var entryAddr uint32
	var err error

	if function != "" {
		entryIndex, err = entry.ModuleFuncIndex(mod, function)
		if err != nil {
			panic(err)
		}

		entryAddr = codeMap.FuncAddrs[entryIndex]
	}

	build.SetupEntryStackFrame(entryIndex, entryAddr)

	var dataConfig = &compile.DataConfig{
		GlobalsMemory:   build.GlobalsMemoryBuffer(),
		MemoryAlignment: build.MemoryAlignment(),
	}

	if err := compile.LoadDataSection(dataConfig, r, mod); err != nil {
		panic(err)
	}

	arc, exe, err = build.FinishArchiveExecutable(arcKey, image.SectionMap{}, nil, nil, nil)
	if err != nil {
		panic(err)
	}

	return
}

func startTest(ctx context.Context, t *testing.T, prog []byte, function string, debugOut io.Writer,
) (*runtimeutil.Executor, *image.Executable, *runtime.Process, debug.InsnMap, compile.Module) {
	executor := runtimeutil.NewExecutor(ctx, &runtime.Config{LibDir: "../lib/gate/runtime"})

	var codeMap debug.InsnMap

	arc, exe, mod := buildTest(executor, "", image.Memory, image.Memory, &codeMap, &codeMap.CallMap, codeMap.Reader(bytes.NewReader(prog)), len(prog), function)
	arc.Close()

	ref := exe.Ref()
	defer ref.Close()

	proc, err := runtime.NewProcess(ctx, executor.Executor, ref, debugOut)
	if err != nil {
		exe.Close()
		executor.Close()
		t.Fatal(err)
	}

	err = proc.Start(exe)
	if err != nil {
		proc.Kill()
		exe.Close()
		executor.Close()
		t.Fatal(err)
	}

	return executor, exe, proc, codeMap, mod
}

func runTest(t *testing.T, prog []byte, function string, debug io.Writer) (output bytes.Buffer) {
	ctx := context.Background()

	executor, exe, proc, _, _ := startTest(ctx, t, prog, function, debug)
	defer proc.Kill()
	defer exe.Close()
	defer executor.Close()

	exit, trapID, err := proc.Serve(ctx, &testServiceRegistry{origin: &output}, nil)
	if err != nil {
		t.Errorf("run error: %v", err)
	} else if trapID != 0 {
		t.Errorf("run %v", trapID)
	} else if exit != 0 {
		t.Errorf("run exit: %d", exit)
	}

	if s := output.String(); len(s) > 0 {
		t.Logf("output: %q", s)
	}
	return
}

func TestRunNop(t *testing.T) {
	runTest(t, testProgNop, "", nil)
}

func testRunHello(t *testing.T, debug io.Writer) {
	output := runTest(t, testProgHello, "main", debug)
	if s := output.String(); s != "hello, world\n" {
		t.Fail()
	}
}

func TestRunHello(t *testing.T) {
	testRunHello(t, os.Stdout)
}

func TestRunHelloNoDebug(t *testing.T) {
	testRunHello(t, nil)
}

func TestRunHelloDebug(t *testing.T) {
	var debug bytes.Buffer
	runTest(t, testProgHelloDebug, "main", &debug)
	s := debug.String()
	t.Logf("debug: %q", s)
	if s != "hello…\nworld\n" {
		t.Fail()
	}
}

func TestRunHelloDebugNoDebug(t *testing.T) {
	runTest(t, testProgHelloDebug, "main", nil)
}

func TestRunSuspend(t *testing.T) {
	ctx := context.Background()

	executor, exe, proc, codeMap, mod := startTest(ctx, t, testProgSuspend, "main", os.Stdout)
	defer proc.Kill()
	defer exe.Close()
	defer executor.Close()

	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	exit, trapID, err := proc.Serve(ctx, &testServiceRegistry{}, nil)
	if err != nil {
		t.Errorf("run error: %v", err)
	} else if trapID == 0 {
		t.Errorf("run exit: %d", exit)
	} else if trapID != trap.Suspended {
		t.Errorf("run %v", trapID)
	}

	if err := exe.CheckTermination(); err != nil {
		t.Errorf("termination: %v", err)
	}

	if false {
		trace, err := exe.Stacktrace(codeMap, mod.FuncTypes())
		if err != nil {
			t.Fatal(err)
		}

		if len(trace) > 0 {
			stacktrace.Fprint(os.Stderr, trace, mod.FuncTypes(), nil, nil)
		}
	}
}

type testServiceRegistry struct {
	origin io.Writer
}

func (services *testServiceRegistry) StartServing(ctx context.Context, config runtime.ServiceConfig, _ []runtime.SuspendedService, send chan<- packet.Buf, recv <-chan packet.Buf,
) (runtime.ServiceDiscoverer, []runtime.ServiceState, error) {
	d := new(testServiceDiscoverer)

	go func() {
		var originInit bool

		for op := range recv {
			code := op.Code()

			d.nameLock.Lock()
			name := d.names[code]
			d.nameLock.Unlock()

			switch name {
			case "origin":
				if !originInit {
					send <- packet.MakeFlow(op.Code(), 0, 100000)
					originInit = true
				}

				switch op.Domain() {
				case packet.DomainData:
					if _, err := services.origin.Write(packet.DataBuf(op).Data()); err != nil {
						panic(err)
					}
				}
			}
		}
	}()

	return d, nil, nil
}

type testServiceDiscoverer struct {
	services []runtime.ServiceState
	nameLock sync.Mutex
	names    []string
}

func (d *testServiceDiscoverer) Discover(names []string) ([]runtime.ServiceState, error) {
	for _, name := range names {
		var s runtime.ServiceState

		switch name {
		case "origin":
			s.SetAvail()
		}

		d.services = append(d.services, s)

		d.nameLock.Lock()
		d.names = append(d.names, name)
		d.nameLock.Unlock()
	}

	return d.services, nil
}

func (d *testServiceDiscoverer) NumServices() int {
	return len(d.services)
}

func (*testServiceDiscoverer) Suspend() []runtime.SuspendedService {
	return nil
}

func (*testServiceDiscoverer) Close() error {
	return nil
}
