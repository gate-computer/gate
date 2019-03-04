// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gate_test

import (
	"bytes"
	"context"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/tsavola/gate/entry"
	"github.com/tsavola/gate/image"
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
	maxTextSize     = 32 * 1024 * 1024
	stackSize       = wa.PageSize
	memorySizeLimit = 128 * 1024 * 1024
)

type executor struct {
	*runtime.Executor
	closed bool
}

func (test *executor) Close() error {
	test.closed = true
	return test.Executor.Close()
}

func newExecutor(ctx context.Context, config *runtime.Config) (tester *executor) {
	if config == nil {
		config = new(runtime.Config)
	}
	if config.LibDir == "" {
		config.LibDir = "lib/gate/runtime"
	}

	actual, err := runtime.NewExecutor(ctx, config)
	if err != nil {
		panic(err)
	}

	tester = &executor{Executor: actual}

	go func() {
		<-tester.Dead()
		if !tester.closed {
			panic("executor died")
		}
	}()

	return
}

type serviceRegistry struct{ origin io.Writer }

func (services serviceRegistry) StartServing(ctx context.Context, config runtime.ServiceConfig, _ []runtime.SuspendedService, send chan<- packet.Buf, recv <-chan packet.Buf,
) (runtime.ServiceDiscoverer, []runtime.ServiceState, error) {
	d := new(serviceDiscoverer)

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

type serviceDiscoverer struct {
	services []runtime.ServiceState
	nameLock sync.Mutex
	names    []string
}

func (d *serviceDiscoverer) Discover(names []string) ([]runtime.ServiceState, error) {
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

func (d *serviceDiscoverer) NumServices() int                  { return len(d.services) }
func (*serviceDiscoverer) Suspend() []runtime.SuspendedService { return nil }
func (*serviceDiscoverer) Close() error                        { return nil }

func prepareBuild(exec *executor, progStorage image.ProgramStorage, instStorage image.InstanceStorage, r compile.Reader, moduleSize int, codeMap *object.CallMap,
) (mod compile.Module, build *image.Build) {
	mod, err := compile.LoadInitialSections(nil, r)
	if err != nil {
		panic(err)
	}

	if err := binding.BindImports(&mod, abi.Imports); err != nil {
		panic(err)
	}

	build, err = image.NewBuild(progStorage, instStorage, moduleSize, maxTextSize, codeMap)
	if err != nil {
		panic(err)
	}

	return
}

func buildInstance(exec *executor, progStorage image.ProgramStorage, instStorage image.InstanceStorage, objectMapper compile.ObjectMapper, codeMap *object.CallMap, r compile.Reader, moduleSize int, function string,
) (prog *image.Program, inst *image.Instance, mod compile.Module) {
	mod, build := prepareBuild(exec, progStorage, instStorage, r, moduleSize, codeMap)
	defer build.Close()

	var codeConfig = &compile.CodeConfig{
		Text:   build.TextBuffer(),
		Mapper: objectMapper,
	}

	err := compile.LoadCodeSection(codeConfig, r, mod)
	if err != nil {
		panic(err)
	}

	// dump.Text(os.Stderr, codeConfig.Text.Bytes(), 0, codeMap.FuncAddrs, nil)

	maxMemorySize := mod.MemorySizeLimit()
	if maxMemorySize > memorySizeLimit {
		maxMemorySize = memorySizeLimit
	}

	var entryIndex uint32
	var entryAddr uint32

	if function != "" {
		entryIndex, err = entry.ModuleFuncIndex(mod, function)
		if err != nil {
			panic(err)
		}

		entryAddr = codeMap.FuncAddrs[entryIndex]
	}

	if err := build.FinishText(stackSize, 0, mod.GlobalsSize(), mod.InitialMemorySize(), maxMemorySize); err != nil {
		panic(err)
	}

	var dataConfig = &compile.DataConfig{
		GlobalsMemory:   build.GlobalsMemoryBuffer(),
		MemoryAlignment: build.MemoryAlignment(),
	}

	if err := compile.LoadDataSection(dataConfig, r, mod); err != nil {
		panic(err)
	}

	prog, err = build.FinishProgram(image.SectionMap{}, nil, nil, nil)
	if err != nil {
		panic(err)
	}

	inst, err = build.FinishInstance(entryIndex, entryAddr)
	if err != nil {
		panic(err)
	}

	return
}

func startInstance(ctx context.Context, t *testing.T, wasm []byte, function string, debugOut io.Writer,
) (*executor, *image.Program, *image.Instance, *runtime.Process, debug.InsnMap, compile.Module) {
	var err error

	executor := newExecutor(ctx, nil)
	defer func() {
		if err != nil {
			executor.Close()
		}
	}()

	var codeMap debug.InsnMap

	prog, inst, mod := buildInstance(executor, image.Memory, image.Memory, &codeMap, &codeMap.CallMap, codeMap.Reader(bytes.NewReader(wasm)), len(wasm), function)
	defer func() {
		if err != nil {
			prog.Close()
			inst.Close()
		}
	}()

	err = prog.Store(fmt.Sprint(crc32.ChecksumIEEE(wasm)))
	if err != nil {
		t.Fatal(err)
	}

	proc, err := runtime.NewProcess(ctx, executor.Executor, debugOut)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err != nil {
			proc.Kill()
		}
	}()

	err = proc.Start(prog, inst)
	if err != nil {
		t.Fatal(err)
	}

	return executor, prog, inst, proc, codeMap, mod
}

func runProgram(t *testing.T, wasm []byte, function string, debug io.Writer) (output bytes.Buffer) {
	ctx := context.Background()

	executor, prog, inst, proc, _, _ := startInstance(ctx, t, wasm, function, debug)
	defer proc.Kill()
	defer inst.Close()
	defer prog.Close()
	defer executor.Close()

	exit, trapID, err := proc.Serve(ctx, serviceRegistry{&output}, nil)
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
	runProgram(t, wasmNop, "", nil)
}

func testRunHello(t *testing.T, debug io.Writer) {
	output := runProgram(t, wasmHello, "main", debug)
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
	runProgram(t, wasmHelloDebug, "main", &debug)
	s := debug.String()
	t.Logf("debug: %q", s)
	if s != "hello…\nworld\n" {
		t.Fail()
	}
}

func TestRunHelloDebugNoDebug(t *testing.T) {
	runProgram(t, wasmHelloDebug, "main", nil)
}

func TestRunSuspend(t *testing.T) {
	ctx := context.Background()

	executor, prog, inst, proc, codeMap, mod := startInstance(ctx, t, wasmSuspend, "main", os.Stdout)
	defer proc.Kill()
	defer inst.Close()
	defer prog.Close()
	defer executor.Close()

	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	exit, trapID, err := proc.Serve(ctx, serviceRegistry{}, nil)
	if err != nil {
		t.Errorf("run error: %v", err)
	} else if trapID == 0 {
		t.Errorf("run exit: %d", exit)
	} else if trapID != trap.Suspended {
		t.Errorf("run %v", trapID)
	}

	if err := inst.CheckMutation(); err != nil {
		t.Errorf("instance state: %v", err)
	}

	if false {
		trace, err := inst.Stacktrace(codeMap, mod.FuncTypes())
		if err != nil {
			t.Fatal(err)
		}

		if len(trace) > 0 {
			stacktrace.Fprint(os.Stderr, trace, mod.FuncTypes(), nil, nil)
		}
	}
}
