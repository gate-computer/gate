// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gate_test

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strconv"
	"strings"
	"testing"

	"github.com/tsavola/gate/image"
	"github.com/tsavola/gate/runtime"
	"github.com/tsavola/wag/object"
	"github.com/tsavola/wag/object/abi"
	"github.com/tsavola/wag/trap"
)

const benchPrepareCount = 32

type nopInstance struct{ *image.Instance }

func (nopInstance) InitRoutine() uint8 { return abi.TextAddrNoFunction }

var benchExecutor = newExecutor(context.Background(), nil)
var benchRegistry = serviceRegistry{new(bytes.Buffer)}
var benchFS image.LocalStorage

type benchDatum struct {
	name string
	wasm []byte
}

var benchData = []benchDatum{
	{"Nop", wasmNop},
	{"Hello", wasmHello},
}

var optionalBenchData = []struct {
	name string
	path string
}{
	{"GainRel", "../gain/target/wasm32-unknown-unknown/release/examples/hello.wasm"},
	{"GainDbg", "../gain/target/wasm32-unknown-unknown/debug/examples/hello.wasm"},
}

func init() {
	for _, x := range optionalBenchData {
		wasm, err := ioutil.ReadFile(x.path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			panic(err)
		}
		benchData = append(benchData, benchDatum{x.name, wasm})
	}

	dir := os.Getenv("GATE_BENCH_FILESYSTEM")
	if dir == "" {
		d := "testdata/filesystem"
		if _, err := os.Stat(d); err == nil {
			dir = d
		} else if !os.IsNotExist(err) {
			panic(err)
		}
	}

	if dir != "" {
		if err := os.RemoveAll(path.Join(dir, "v0/program")); err != nil {
			panic(err)
		}
		benchFS = image.NewFilesystem(dir)
	}
}

func executeInstance(ctx context.Context, prog runtime.ProgramCode, inst runtime.ProgramState,
) (exit int, trapID trap.ID, err error) {
	proc, err := runtime.NewProcess(ctx, benchExecutor.Executor, nil)
	if err != nil {
		return
	}
	defer proc.Kill()

	err = proc.Start(prog, inst)
	if err != nil {
		return
	}

	return proc.Serve(ctx, benchRegistry, nil)
}

func executeProgram(ctx context.Context, instStorage image.InstanceStorage, prog *image.Program,
) (exit int, trapID trap.ID, err error) {
	proc, err := runtime.NewProcess(ctx, benchExecutor.Executor, nil)
	if err != nil {
		return
	}
	defer proc.Kill()

	inst, err := image.NewInstance(instStorage, prog, stackSize, 0, 0)
	if err != nil {
		return
	}
	defer inst.Close()

	err = proc.Start(prog, nopInstance{inst})
	if err != nil {
		return
	}

	return proc.Serve(ctx, benchRegistry, nil)
}

func BenchmarkBuildMem(b *testing.B) {
	benchBuild(b, image.Memory, image.Memory)
}

func BenchmarkBuildMemPrep(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	benchBuild(b, image.PreparePrograms(ctx, image.Memory, benchPrepareCount), image.PrepareInstances(ctx, image.Memory, benchPrepareCount))
}

func BenchmarkBuildFS(b *testing.B) {
	if benchFS == nil {
		b.Skip("filesystem backend unavailable")
	}

	benchBuild(b, benchFS, benchFS)
}

func benchBuild(b *testing.B, progStorage image.ProgramStorage, instStorage image.InstanceStorage) {
	for _, x := range benchData {
		wasm := x.wasm
		b.Run(x.name, func(b *testing.B) {
			var codeMap object.CallMap

			for i := 0; i < b.N; i++ {
				codeMap.FuncAddrs = codeMap.FuncAddrs[:0]
				codeMap.CallSites = codeMap.CallSites[:0]

				prog, inst, _ := buildInstance(benchExecutor, progStorage, instStorage, &codeMap, &codeMap, bytes.NewReader(wasm), len(wasm), "")
				inst.Close()
				prog.Close()
			}
		})
	}
}

func BenchmarkBuildStore(b *testing.B) {
	if benchFS == nil {
		b.Skip("filesystem backend unavailable")
	}

	var progStorage image.ProgramStorage = benchFS
	var instStorage image.InstanceStorage = benchFS

	prefix := fmt.Sprintf("%s.%d.", strings.Replace(b.Name(), "/", "-", -1), b.N)

	var codeMap object.CallMap

	for i := 0; i < b.N; i++ {
		codeMap.FuncAddrs = codeMap.FuncAddrs[:0]
		codeMap.CallSites = codeMap.CallSites[:0]

		prog, inst, _ := buildInstance(benchExecutor, progStorage, instStorage, &codeMap, &codeMap, bytes.NewReader(wasmNop), len(wasmNop), "")
		err := prog.Store(prefix + strconv.Itoa(i))
		inst.Close()
		prog.Close()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkExecInstMem(b *testing.B) {
	benchExecInst(b, image.Memory, image.Memory)
}

func BenchmarkExecInstFS(b *testing.B) {
	if benchFS == nil {
		b.Skip("filesystem backend unavailable")
	}

	benchExecInst(b, benchFS, benchFS)
}

func benchExecInst(b *testing.B, progStorage image.ProgramStorage, instStorage image.InstanceStorage) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var codeMap object.CallMap

	prog, instProto, _ := buildInstance(benchExecutor, progStorage, instStorage, &codeMap, &codeMap, bytes.NewReader(wasmNop), len(wasmNop), "")
	defer prog.Close()
	defer instProto.Close()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		instClone := *instProto // Hack to work around state invalidation.

		_, trapID, err := executeInstance(ctx, prog, nopInstance{&instClone})
		if err != nil {
			b.Fatal(err)
		}
		if trapID != trap.NoFunction {
			b.Error(trapID)
		}
	}

	b.StopTimer()
}

func BenchmarkExecProgMem(b *testing.B) {
	benchExecProg(b, image.Memory, image.Memory)
}

func BenchmarkExecProgMemPrep(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	benchExecProg(b, image.Memory, image.PrepareInstances(ctx, image.Memory, benchPrepareCount))
}

func BenchmarkExecProgFS(b *testing.B) {
	if benchFS == nil {
		b.Skip("filesystem backend unavailable")
	}

	benchExecProg(b, benchFS, benchFS)
}

func benchExecProg(b *testing.B, progStorage image.ProgramStorage, instStorage image.InstanceStorage) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, x := range benchData {
		wasm := x.wasm
		b.Run(x.name, func(b *testing.B) {
			var codeMap object.CallMap

			prog, inst, _ := buildInstance(benchExecutor, progStorage, instStorage, &codeMap, &codeMap, bytes.NewReader(wasm), len(wasm), "")
			defer prog.Close()
			inst.Close()

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_, trapID, err := executeProgram(ctx, instStorage, prog)
				if err != nil {
					b.Fatal(err)
				}
				if trapID != trap.NoFunction {
					b.Error(trapID)
				}
			}

			b.StopTimer()
		})
	}
}
