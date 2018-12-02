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
	"github.com/tsavola/wag/compile"
	"github.com/tsavola/wag/object/debug"
	"github.com/tsavola/wag/trap"
	"github.com/tsavola/wag/wa"
)

const (
	testMaxTextSize   = 32 * 1024 * 1024
	testMaxMemorySize = 128 * 1024 * 1024
	testStackSize     = wa.PageSize
)

type readWriter struct {
	io.Reader
	io.Writer
}

var (
	testProgNop        = runtimeutil.MustReadFile("../testdata/nop.wasm")
	testProgHello      = runtimeutil.MustReadFile("../testdata/hello.wasm")
	testProgHelloDebug = runtimeutil.MustReadFile("../testdata/hello-debug.wasm")
	testProgSuspend    = runtimeutil.MustReadFile("../testdata/suspend.wasm")
)

func prepareTest(exec *runtimeutil.Executor, r *bytes.Reader,
) (mod *compile.Module, build *image.Build, config *image.BuildConfig) {
	mod, err := compile.LoadInitialSections(nil, r)
	if err != nil {
		panic(err)
	}

	if err := abi.BindImports(mod); err != nil {
		panic(err)
	}

	ref, err := image.NewExecutableRef(image.Memory)
	if err != nil {
		panic(err)
	}
	defer ref.Close()

	build = image.NewBuild(ref)

	config = &image.BuildConfig{
		Config: image.Config{
			MaxTextSize:   testMaxTextSize,
			StackSize:     testStackSize,
			MaxMemorySize: mod.MemorySizeLimit(),
		},
		GlobalsSize: mod.GlobalsSize(),
		MemorySize:  mod.InitialMemorySize(),
	}

	if config.MaxMemorySize > testMaxMemorySize {
		config.MaxMemorySize = testMaxMemorySize
	}

	if err := build.Configure(config); err != nil {
		panic(err)
	}
	return
}

func compileTest(exec *runtimeutil.Executor, prog []byte, function string,
) (exe *image.Executable, mod *compile.Module) {
	r := bytes.NewReader(prog)

	mod, build, buildConfig := prepareTest(exec, r)
	defer build.Close()

	var codeMap = new(debug.InsnMap)
	var codeConfig = &compile.CodeConfig{
		Text:   build.TextBuffer(),
		Mapper: codeMap,
	}

	if err := compile.LoadCodeSection(codeConfig, r, mod); err != nil {
		panic(err)
	}

	buildConfig.MaxTextSize = len(codeConfig.Text.Bytes())

	if err := build.Configure(buildConfig); err != nil {
		panic(err)
	}

	var entryAddr uint32

	if function != "" {
		entryIndex, err := entry.FuncIndex(mod, function)
		if err != nil {
			panic(err)
		}

		entryAddr = codeMap.FuncAddrs[entryIndex]
	}

	build.SetupEntryStackFrame(entryAddr)

	var dataConfig = &compile.DataConfig{
		GlobalsMemory:   build.GlobalsMemoryBuffer(),
		MemoryAlignment: os.Getpagesize(),
	}

	if err := compile.LoadDataSection(dataConfig, r, mod); err != nil {
		panic(err)
	}

	// var nameSection = new(section.NameSection)
	// var nameConfig = &compile.Config{
	// 	CustomSectionLoader: section.CustomLoaders{
	// 		section.CustomName: nameSection.Load,
	// 	}.Load,
	// }
	//
	// if err := compile.LoadCustomSections(nameConfig, r); err != nil {
	// 	panic(err)
	// }
	//
	// dump.Text(os.Stderr, text, 0, codeMap, nameSection)

	exe, err := build.Executable()
	if err != nil {
		panic(err)
	}

	return
}

func startTest(ctx context.Context, t *testing.T, prog []byte, function string, debug io.Writer,
) (*runtimeutil.Executor, *image.Executable, *runtime.Process) {
	executor := runtimeutil.NewExecutor(ctx, &runtime.Config{LibDir: "../lib/gate/runtime"})

	exe, _ := compileTest(executor, prog, function)

	ref := exe.Ref()
	defer ref.Close()

	proc, err := runtime.NewProcess(ctx, executor.Executor, ref, debug)
	if err != nil {
		exe.Close()
		executor.Close()
		t.Fatal(err)
	}

	err = proc.Start(exe, runtime.InitStart)
	if err != nil {
		proc.Kill()
		exe.Close()
		executor.Close()
		t.Fatal(err)
	}

	return executor, exe, proc
}

func runTest(t *testing.T, prog []byte, function string, debug io.Writer) (output bytes.Buffer) {
	ctx := context.Background()

	executor, exe, proc := startTest(ctx, t, prog, function, debug)
	defer proc.Kill()
	defer exe.Close()
	defer executor.Close()

	exit, trapID, err := proc.Serve(ctx, &testServiceRegistry{origin: &output})
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

	executor, exe, proc := startTest(ctx, t, testProgSuspend, "main", os.Stdout)
	defer proc.Kill()
	defer exe.Close()
	defer executor.Close()

	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	exit, trapID, err := proc.Serve(ctx, &testServiceRegistry{})
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

	return
}

type testServiceRegistry struct {
	origin io.Writer
}

func (services *testServiceRegistry) StartServing(ctx context.Context, config *runtime.ServiceConfig, send chan<- packet.Buf, recv <-chan packet.Buf) runtime.ServiceDiscoverer {
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

	return d
}

type testServiceDiscoverer struct {
	services []runtime.ServiceState
	nameLock sync.Mutex
	names    []string
}

func (d *testServiceDiscoverer) Discover(names []string) []runtime.ServiceState {
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

	return d.services
}

func (d *testServiceDiscoverer) NumServices() int {
	return len(d.services)
}
