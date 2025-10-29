// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gate_test

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"gate.computer/gate/image"
	"gate.computer/gate/runtime"
	"gate.computer/gate/trap"
	"gate.computer/wag/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	. "import.name/type/context"
)

const benchPrepareCount = 32

var (
	benchExecInit sync.Once
	benchExecutor *executor
	benchRegistry = serviceRegistry{origin: nopCloser{new(bytes.Buffer)}}
)

func getBenchExecutor() *executor {
	benchExecInit.Do(func() {
		benchExecutor = newExecutor()
	})
	return benchExecutor
}

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
	{"Doom", "../wag-bench/003.wasm"},
}

func init() {
	for _, x := range optionalBenchData {
		wasm, err := os.ReadFile(x.path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			panic(err)
		}
		benchData = append(benchData, benchDatum{x.name, wasm})
	}
}

func executeInstance(ctx Context, prog runtime.ProgramCode, inst runtime.ProgramState) (runtime.Result, trap.ID, error) {
	proc, err := getBenchExecutor().NewProcess(ctx)
	if err != nil {
		return runtime.Result{}, trap.InternalError, err
	}
	defer proc.Kill()

	policy := runtime.ProcessPolicy{
		TimeResolution: time.Millisecond,
	}

	if err := proc.Start(prog, inst, policy); err != nil {
		return runtime.Result{}, trap.InternalError, err
	}

	result, trapID, _, err := proc.Serve(ctx, benchRegistry, nil)
	return result, trapID, err
}

func executeProgram(ctx Context, prog *image.Program) (runtime.Result, trap.ID, error) {
	proc, err := getBenchExecutor().NewProcess(ctx)
	if err != nil {
		return runtime.Result{}, trap.InternalError, err
	}
	defer proc.Kill()

	inst, err := image.NewInstance(prog, 0x7fff0000, stackSize, -1)
	if err != nil {
		return runtime.Result{}, trap.InternalError, err
	}
	defer inst.Close()

	if err := proc.Start(prog, inst, runtime.ProcessPolicy{}); err != nil {
		return runtime.Result{}, trap.InternalError, err
	}

	result, trapID, _, err := proc.Serve(ctx, benchRegistry, nil)
	return result, trapID, err
}

func BenchmarkBuildMem(b *testing.B) {
	benchBuild(b, image.Memory)
}

func BenchmarkBuildMemPrep(b *testing.B) {
	benchBuild(b, image.CombinedStorage(image.PreparePrograms(b.Context(), image.Memory, benchPrepareCount), image.PrepareInstances(b.Context(), image.Memory, benchPrepareCount)))
}

func BenchmarkBuildFS(b *testing.B) {
	if testFS == nil {
		b.Skip("test filesystem not specified")
	}

	benchBuild(b, testFS)
}

func benchBuild(b *testing.B, storage image.Storage) {
	for _, x := range benchData {
		wasm := x.wasm
		b.Run(x.name, func(b *testing.B) {
			var codeMap object.CallMap

			for b.Loop() {
				codeMap.FuncAddrs = codeMap.FuncAddrs[:0]
				codeMap.CallSites = codeMap.CallSites[:0]

				prog, inst, _ := buildInstance(getBenchExecutor(), storage, &codeMap, wasm, len(wasm), "", false)
				inst.Close()
				prog.Close()
			}
		})
	}
}

func BenchmarkBuildStore(b *testing.B) {
	if testFS == nil {
		b.Skip("test filesystem not specified")
	}

	prefix := fmt.Sprintf("%s.%d.", strings.Replace(b.Name(), "/", "-", -1), b.N)

	var codeMap object.CallMap

	for i := range b.N {
		codeMap.FuncAddrs = codeMap.FuncAddrs[:0]
		codeMap.CallSites = codeMap.CallSites[:0]

		prog, inst, _ := buildInstance(getBenchExecutor(), testFS, &codeMap, wasmNop, len(wasmNop), "", false)
		err := prog.Store(prefix + strconv.Itoa(i))
		inst.Close()
		prog.Close()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkExecInstMem(b *testing.B) {
	benchExecInst(b, image.Memory)
}

func BenchmarkExecInstFS(b *testing.B) {
	if testFS == nil {
		b.Skip("test filesystem not specified")
	}

	benchExecInst(b, testFS)
}

func benchExecInst(b *testing.B, storage image.Storage) {
	var codeMap object.CallMap

	prog, instProto, _ := buildInstance(getBenchExecutor(), storage, &codeMap, wasmNop, len(wasmNop), "", false)
	defer prog.Close()
	defer instProto.Close()

	b.ResetTimer()

	for b.Loop() {
		instClone := *instProto // Hack to work around state invalidation.

		result, trapID, err := executeInstance(b.Context(), prog, &instClone)
		require.NoError(b, err)
		assert.Equal(b, trapID, trap.Exit)
		assert.False(b, result.Terminated())
		assert.Equal(b, result.Value(), runtime.ResultSuccess)
	}

	b.StopTimer()
}

func BenchmarkExecProgMem(b *testing.B) {
	benchExecProg(b, image.Memory)
}

func BenchmarkExecProgMemPrep(b *testing.B) {
	benchExecProg(b, image.CombinedStorage(image.Memory, image.PrepareInstances(b.Context(), image.Memory, benchPrepareCount)))
}

func BenchmarkExecProgFS(b *testing.B) {
	if testFS == nil {
		b.Skip("test filesystem not specified")
	}

	benchExecProg(b, testFS)
}

func BenchmarkExecProgPersistMem(b *testing.B) {
	if testFS == nil {
		b.Skip("test filesystem not specified")
	}

	s := image.CombinedStorage(testFS, image.PersistentMemory(testFS))
	benchExecProg(b, s)
}

func benchExecProg(b *testing.B, storage image.Storage) {
	for _, x := range benchData {
		wasm := x.wasm
		b.Run(x.name, func(b *testing.B) {
			var codeMap object.CallMap

			prog, inst, _ := buildInstance(getBenchExecutor(), storage, &codeMap, wasm, len(wasm), "", false)
			defer prog.Close()
			inst.Close()

			b.ResetTimer()

			for b.Loop() {
				result, trapID, err := executeProgram(b.Context(), prog)
				require.NoError(b, err)
				assert.Equal(b, trapID, trap.Exit)
				assert.False(b, result.Terminated())
				assert.Equal(b, result.Value(), runtime.ResultSuccess)
			}

			b.StopTimer()
		})
	}
}

type nopCloser struct {
	io.Writer
}

func (nopCloser) Close() error {
	return nil
}
