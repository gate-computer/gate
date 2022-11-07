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
	"io/ioutil"
	"os"
	"path"
	"regexp"
	goruntime "runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"gate.computer/gate/image"
	"gate.computer/gate/packet"
	"gate.computer/gate/runtime"
	"gate.computer/gate/runtime/abi"
	"gate.computer/gate/runtime/container"
	"gate.computer/gate/service"
	"gate.computer/gate/service/origin"
	"gate.computer/gate/snapshot"
	"gate.computer/gate/trap"
	internalbuild "gate.computer/internal/build"
	"gate.computer/wag/binding"
	"gate.computer/wag/compile"
	"gate.computer/wag/object"
	objectabi "gate.computer/wag/object/abi"
	"gate.computer/wag/object/debug/dump"
	"gate.computer/wag/object/stack/stacktrace"
	"gate.computer/wag/wa"
	"import.name/lock"
)

const (
	dumpText = false
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

func newExecutor() (tester *executor) {
	actual, err := runtime.NewExecutor(&runtime.Config{
		Container: container.Config{
			Namespace: testNamespaceConfig,
			ExecDir:   testExecDir,
		},
	})
	if err != nil {
		panic(err)
	}

	tester = &executor{Executor: actual}

	go func() {
		<-tester.Dead()
		time.Sleep(time.Second)
		if !tester.closed {
			time.Sleep(time.Second)
			panic("executor died")
		}
	}()

	return
}

type serviceRegistry struct {
	origin   io.WriteCloser
	originMu *sync.Mutex
}

func (services serviceRegistry) CreateServer(ctx context.Context, config runtime.ServiceConfig, _ []snapshot.Service, send chan<- packet.Thunk) (runtime.InstanceServer, []runtime.ServiceState, <-chan error, error) {
	d := &serviceDiscoverer{
		registry: services,
		config:   config,
	}
	return d, nil, make(chan error), nil
}

type serviceDiscoverer struct {
	registry serviceRegistry
	config   runtime.ServiceConfig
	services []runtime.ServiceState
	nameMu   sync.Mutex
	names    []string

	origin service.Instance
}

func (*serviceDiscoverer) Start(context.Context, chan<- packet.Thunk) error {
	return nil
}

func (d *serviceDiscoverer) Discover(ctx context.Context, names []string) ([]runtime.ServiceState, error) {
	for _, name := range names {
		var s runtime.ServiceState

		switch name {
		case "origin":
			s.SetAvail()
		}

		d.services = append(d.services, s)

		lock.Guard(&d.nameMu, func() {
			d.names = append(d.names, name)
		})
	}

	return d.services, nil
}

func (d *serviceDiscoverer) Handle(ctx context.Context, send chan<- packet.Thunk, p packet.Buf) (packet.Buf, error) {
	var name string
	lock.Guard(&d.nameMu, func() {
		name = d.names[p.Code()]
	})

	switch name {
	case "origin":
		if d.origin == nil {
			connector := origin.New(nil)
			go func() {
				defer connector.Close()

				if d.registry.originMu != nil {
					d.registry.originMu.Lock()
					defer d.registry.originMu.Unlock()
				}

				if f := connector.Connect(context.Background()); f != nil {
					f(context.Background(), bytes.NewReader(nil), d.registry.origin)
				}
			}()

			inst, err := connector.CreateInstance(ctx, service.InstanceConfig{
				Service: packet.Service{
					Code:        p.Code(),
					MaxSendSize: d.config.MaxSendSize,
				},
			}, nil)
			if err != nil {
				return nil, err
			}
			d.origin = inst

			if err := d.origin.Ready(ctx); err != nil {
				return nil, err
			}
			if err := d.origin.Start(ctx, send, func(e error) { panic(e) }); err != nil {
				return nil, err
			}
		}

		return d.origin.Handle(ctx, send, p)
	}

	return nil, nil
}

func (d *serviceDiscoverer) Shutdown(ctx context.Context, suspend bool) ([]snapshot.Service, error) {
	if d.origin == nil {
		return nil, nil
	}

	b, err := d.origin.Shutdown(ctx, suspend)
	return []snapshot.Service{{Name: "origin", Buffer: b}}, err
}

var testFS *image.Filesystem

func init() {
	dir := os.Getenv("GATE_TEST_FILESYSTEM")
	if dir == "" {
		d := "../testdata/filesystem"
		if _, err := os.Stat(d); err == nil {
			dir = d
		} else if !os.IsNotExist(err) {
			panic(err)
		}
	}

	if dir != "" {
		if err := os.RemoveAll(path.Join(dir, "program")); err != nil {
			panic(err)
		}
		if err := os.RemoveAll(path.Join(dir, "instance")); err != nil {
			panic(err)
		}
		fs, err := image.NewFilesystem(dir)
		if err != nil {
			panic(err)
		}
		testFS = fs
	}
}

func prepareBuild(exec *executor, storage image.Storage, config compile.Config, wasm []byte, moduleSize int, codeMap *object.CallMap) (compile.Loader, compile.Module, *image.Build) {
	r := compile.NewLoader(bytes.NewReader(wasm))

	mod, err := compile.LoadInitialSections(&compile.ModuleConfig{MaxExports: 100, Config: config}, r)
	if err != nil {
		panic(err)
	}

	build, err := image.NewBuild(storage, moduleSize, maxTextSize, codeMap, true)
	if err != nil {
		panic(err)
	}

	if err := binding.BindImports(&mod, build.ImportResolver()); err != nil {
		panic(err)
	}

	return r, mod, build
}

func buildInstance(exec *executor, storage image.Storage, codeMap *object.CallMap, wasm []byte, moduleSize int, function string, persistent bool) (*image.Program, *image.Instance, compile.Module) {
	var config compile.Config
	var sectionMap image.SectionMap

	if persistent {
		config.ModuleMapper = &sectionMap
	}

	r, mod, build := prepareBuild(exec, storage, config, wasm, moduleSize, codeMap)
	defer build.Close()

	if persistent {
		if _, err := build.ModuleWriter().Write(wasm); err != nil {
			panic(err)
		}
	}

	codeConfig := &compile.CodeConfig{
		Text:   build.TextBuffer(),
		Mapper: codeMap,
		Config: config,
	}

	err := compile.LoadCodeSection(codeConfig, r, mod, abi.Library())
	if err != nil {
		panic(err)
	}

	var textDump []byte
	if dumpText {
		textDump = append([]byte(nil), codeConfig.Text.Bytes()...)
	}

	entryIndex, err := internalbuild.ResolveEntryFunc(mod, function, false)
	if err != nil {
		panic(err)
	}

	if err := build.FinishText(stackSize, 0, mod.GlobalsSize(), mod.InitialMemorySize()); err != nil {
		panic(err)
	}

	dataConfig := &compile.DataConfig{
		GlobalsMemory:   build.GlobalsMemoryBuffer(),
		MemoryAlignment: build.MemoryAlignment(),
		Config:          config,
	}

	if err := compile.LoadDataSection(dataConfig, r, mod); err != nil {
		panic(err)
	}

	if persistent || dumpText {
		if err := compile.LoadCustomSections(&config, r); err != nil {
			panic(err)
		}
	}

	if dumpText {
		if err := dump.Text(os.Stderr, textDump, 0, codeMap.FuncAddrs, nil); err != nil {
			panic(err)
		}
	}

	startIndex := -1
	if index, found := mod.StartFunc(); found {
		startIndex = int(index)
	}

	prog, err := build.FinishProgram(sectionMap, mod, startIndex, true, nil, 0)
	if err != nil {
		panic(err)
	}

	memLimit := mod.MemorySizeLimit()
	if memLimit < 0 || memLimit > memorySizeLimit {
		memLimit = memorySizeLimit
	}

	inst, err := build.FinishInstance(prog, memLimit, entryIndex)
	if err != nil {
		panic(err)
	}

	return prog, inst, mod
}

func startInstance(ctx context.Context, t *testing.T, storage image.Storage, wasm []byte, function string, debugOut io.Writer) (*executor, *image.Program, *image.Instance, *runtime.Process, *object.CallMap, compile.Module) {
	var ok bool

	executor := newExecutor()
	defer func() {
		if !ok {
			executor.Close()
		}
	}()

	codeMap := new(object.CallMap)

	prog, inst, mod := buildInstance(executor, storage, codeMap, wasm, len(wasm), function, true)
	defer func() {
		if !ok {
			prog.Close()
			inst.Close()
		}
	}()

	if err := prog.Store(fmt.Sprint(crc32.ChecksumIEEE(wasm))); err != nil {
		t.Fatal(err)
	}

	proc, err := executor.NewProcess(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if !ok {
			proc.Kill()
		}
	}()

	policy := runtime.ProcessPolicy{
		TimeResolution: time.Microsecond,
		DebugLog:       debugOut,
	}

	if err := proc.Start(prog, inst, policy); err != nil {
		t.Fatal(err)
	}

	ok = true
	return executor, prog, inst, proc, codeMap, mod
}

func runProgram(t *testing.T, wasm []byte, function string, debug io.Writer, expectTrap trap.ID) string {
	t.Helper()

	ctx := context.Background()

	executor, prog, inst, proc, textMap, mod := startInstance(ctx, t, image.Memory, wasm, function, debug)
	defer proc.Kill()
	defer inst.Close()
	defer prog.Close()
	defer executor.Close()

	var output bytes.Buffer
	var outputMu sync.Mutex

	result, trapID, err := proc.Serve(ctx, serviceRegistry{nopCloser{&output}, &outputMu}, nil)
	if err != nil {
		t.Errorf("run error: %v", err)
	} else {
		if trapID != expectTrap {
			t.Errorf("run %v", trapID)
		}
		if trapID == trap.Exit && result.Value() != runtime.ResultSuccess {
			t.Errorf("run result: %s", result)
		}
		if testing.Verbose() {
			trace, err := inst.Stacktrace(textMap, mod.FuncTypes())
			if err == nil {
				err = stacktrace.Fprint(os.Stderr, trace, mod.FuncTypes(), nil, nil)
			}
			if err != nil {
				t.Error(err)
			}
		}
	}

	outputMu.Lock()
	defer outputMu.Unlock()

	return output.String()
}

func TestABI(t *testing.T) {
	src, err := ioutil.ReadFile("../testdata/abi.cpp")
	if err != nil {
		t.Fatal(err)
	}

	re := regexp.MustCompile(`^([A-Za-z0-9_]+)\s*\(\s*([A-Za-z0-9_]*)\s*\)`)

	for _, line := range strings.Split(string(src), "\n") {
		m := re.FindStringSubmatch(line)
		if len(m) == 0 {
			continue
		}

		switch m[1] {
		case "TEST":
			testABI(t, m[2])
		case "TEST_TRAP":
			testABITrap(t, m[2])
		default:
			t.Error(m[0])
		}
	}
}

func testABI(t *testing.T, name string) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		var debug bytes.Buffer
		runProgram(t, wasmABI, "test_"+name, &debug, trap.Exit)
		if s := debug.String(); s != "PASS\n" {
			t.Errorf("output: %q", s)
		}
	})
}

func testABITrap(t *testing.T, name string) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		var debug bytes.Buffer
		runProgram(t, wasmABI, "testtrap_"+name, &debug, trap.ABIDeficiency)
		if s := debug.String(); s != "" {
			t.Errorf("output: %q", s)
		}
	})
}

func TestRunNop(t *testing.T) {
	runProgram(t, wasmNop, "", nil, trap.Exit)
}

func testRunHello(t *testing.T, debug io.Writer) {
	s := runProgram(t, wasmHello, "greet", debug, trap.Exit)
	if s != "hello, world\n" {
		t.Errorf("output: %q", s)
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
	runProgram(t, wasmHelloDebug, "debug", &debug, trap.Exit)
	s := debug.String()
	if s != "hello, world\n" {
		t.Errorf("debug: %q", s)
	}
}

func TestRunHelloDebugNoDebug(t *testing.T) {
	runProgram(t, wasmHelloDebug, "debug", nil, trap.Exit)
}

func TestRunSuspendMem(t *testing.T) {
	testRunSuspend(t, image.Memory, objectabi.TextAddrNoFunction)
}

func TestRunSuspendFS(t *testing.T) {
	if testFS == nil {
		t.Skip("test filesystem not specified")
	}

	testRunSuspend(t, testFS, objectabi.TextAddrResume)
}

func TestRunSuspendPersistMem(t *testing.T) {
	if testFS == nil {
		t.Skip("test filesystem not specified")
	}

	s := image.CombinedStorage(testFS, image.PersistentMemory(testFS))
	testRunSuspend(t, s, objectabi.TextAddrResume)
}

func testRunSuspend(t *testing.T, storage image.Storage, expectInitRoutine uint32) {
	ctx := context.Background()

	executor, prog, inst, proc, codeMap, mod := startInstance(ctx, t, storage, wasmSuspend, "loop", os.Stdout)
	defer proc.Kill()
	defer inst.Close()
	defer prog.Close()
	defer executor.Close()

	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	var buffers snapshot.Buffers

	exit, trapID, err := proc.Serve(ctx, serviceRegistry{}, &buffers)
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

	if err := inst.Store(t.Name(), t.Name(), prog); err != nil {
		t.Fatal(err)
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

	if false {
		prog2, err := image.Snapshot(prog, inst, buffers, true)
		if err != nil {
			t.Fatal(err)
		}
		defer prog2.Close()

		data, err := ioutil.ReadAll(prog2.NewModuleReader())
		if err != nil {
			t.Fatal(err)
		}

		filename := fmt.Sprintf("../testdata/%s.%s.wasm", t.Name(), goruntime.GOARCH)

		if err := ioutil.WriteFile(filename, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRandomSeed(t *testing.T) {
	values := make([][2]uint64, 10)

	for i := 0; i < len(values); i++ {
		var debug bytes.Buffer
		runProgram(t, wasmRandomSeed, "dump", &debug, trap.Exit)
		for j, s := range strings.Split(debug.String(), " ") {
			n, err := strconv.ParseUint(strings.TrimSpace(s), 16, 64)
			if err != nil {
				t.Fatal(err)
			}
			values[i][j] = n
		}
	}

	for i := 0; i < len(values); i++ {
		for j := 0; j < len(values); j++ {
			if i != j && values[i] == values[j] {
				t.Fatal(values[i])
			}
		}
	}
}

func TestRandomDeficiency(t *testing.T) {
	testRandomDeficiency(t, "toomuch")
}

func TestRandomDeficiency2(t *testing.T) {
	testRandomDeficiency(t, "toomuch2")
}

func testRandomDeficiency(t *testing.T, function string) {
	var debug bytes.Buffer
	runProgram(t, wasmRandomSeed, function, &debug, trap.ABIDeficiency)
	if s := debug.String(); s != "ping\n" {
		t.Errorf("debug: %q", s)
	}
}

func TestTime(t *testing.T) {
	runProgram(t, wasmTime, "check", os.Stderr, trap.Exit)
}
