// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime_test

import (
	"bytes"
	"context"
	"os"
	"strconv"
	"testing"

	"github.com/tsavola/gate/image"
	"github.com/tsavola/gate/internal/test/runtimeutil"
	"github.com/tsavola/gate/runtime"
	"github.com/tsavola/wag/object/abi"
	"github.com/tsavola/wag/object/stack"
	"github.com/tsavola/wag/trap"
)

type storage interface {
	image.BackingStore
	image.Storage
}

const noFunction = runtime.InitRoutine(abi.TextAddrNoFunction)

var benchExecutor = runtimeutil.NewExecutor(context.Background(), &runtime.Config{
	LibDir: "../lib/gate/runtime",
})

var benchRegistry = &testServiceRegistry{new(bytes.Buffer)}

var (
	benchProgGainRelease = runtimeutil.MustReadFile("../../gain/target/wasm32-unknown-unknown/release/examples/hello.wasm")
	benchProgGainDebug   = runtimeutil.MustReadFile("../../gain/target/wasm32-unknown-unknown/debug/examples/hello.wasm")
)

var benchFs storage

func init() {
	if dir := os.Getenv("GATE_BENCH_IMAGE_FS"); dir != "" {
		var pagesize int
		var err error
		if s := os.Getenv("GATE_BENCH_IMAGE_PAGESIZE"); s != "" {
			pagesize, err = strconv.Atoi(s)
			if err != nil {
				panic(err)
			}
		}
		benchFs = image.NewFilesystem(dir, pagesize)
	}
}

func executeRefBench(ctx context.Context, exe *image.Executable, ref image.ExecutableRef,
) (exit int, trapID trap.ID, err error) {
	proc, err := runtime.NewProcess(ctx, benchExecutor.Executor, ref, nil)
	if err != nil {
		return
	}
	defer proc.Kill()

	err = proc.Start(exe, noFunction)
	if err != nil {
		return
	}

	return proc.Serve(ctx, benchRegistry)
}

func executeArBench(ctx context.Context, store image.BackingStore, ar image.Archive) (exit int, trapID trap.ID, err error) {
	ref, err := image.NewExecutableRef(store)
	if err != nil {
		return
	}
	defer ref.Close()

	var config = &image.Config{
		MaxTextSize:   testMaxTextSize,
		StackSize:     testStackSize,
		MaxMemorySize: testMaxMemorySize,
	}

	exe, err := image.LoadExecutable(ctx, ref, config, ar, stack.EntryFrame(0, nil))
	if err != nil {
		return
	}
	defer exe.Close()

	proc, err := runtime.NewProcess(ctx, benchExecutor.Executor, ref, nil)
	if err != nil {
		return
	}
	defer proc.Kill()

	err = proc.Start(exe, noFunction)
	if err != nil {
		return
	}

	return proc.Serve(ctx, benchRegistry)
}

func BenchmarkMemCompileNop(b *testing.B)         { benchCompile(b, image.Memory, testProgNop) }
func BenchmarkMemCompileHello(b *testing.B)       { benchCompile(b, image.Memory, testProgHello) }
func BenchmarkMemCompileGainRelease(b *testing.B) { benchCompile(b, image.Memory, benchProgGainRelease) }
func BenchmarkMemCompileGainDebug(b *testing.B)   { benchCompile(b, image.Memory, benchProgGainDebug) }
func BenchmarkMemExecuteNopRef(b *testing.B)      { benchExecuteRef(b, image.Memory, testProgNop) }
func BenchmarkMemExecuteNopAr(b *testing.B)       { benchExecuteAr(b, image.Memory, testProgNop) }
func BenchmarkMemExecuteHelloAr(b *testing.B)     { benchExecuteAr(b, image.Memory, testProgHello) }
func BenchmarkMemExecuteGainDebugAr(b *testing.B) { benchExecuteAr(b, image.Memory, benchProgGainDebug) }

func BenchmarkFsCompileNop(b *testing.B)         { benchCompile(b, benchFs, testProgNop) }
func BenchmarkFsCompileHello(b *testing.B)       { benchCompile(b, benchFs, testProgHello) }
func BenchmarkFsCompileGainRelease(b *testing.B) { benchCompile(b, benchFs, benchProgGainRelease) }
func BenchmarkFsCompileGainDebug(b *testing.B)   { benchCompile(b, benchFs, benchProgGainDebug) }
func BenchmarkFsExecuteNopRef(b *testing.B)      { benchExecuteRef(b, benchFs, testProgNop) }
func BenchmarkFsExecuteNopAr(b *testing.B)       { benchExecuteAr(b, benchFs, testProgNop) }
func BenchmarkFsExecuteHelloAr(b *testing.B)     { benchExecuteAr(b, benchFs, testProgHello) }
func BenchmarkFsExecuteGainDebugAr(b *testing.B) { benchExecuteAr(b, benchFs, benchProgGainDebug) }

func benchCompile(b *testing.B, store image.BackingStore, prog []byte) {
	if store == nil {
		b.Skip("nil")
	}

	for i := 0; i < b.N; i++ {
		exe, _ := compileTest(benchExecutor, store, prog, "")
		exe.Close()
	}
}

func benchExecuteRef(b *testing.B, store image.BackingStore, prog []byte) {
	if store == nil {
		b.Skip("nil")
	}

	ctx := context.Background()

	exe, _ := compileTest(benchExecutor, store, prog, "")
	defer exe.Close()

	ref := exe.Ref()
	defer ref.Close()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, trapID, err := executeRefBench(ctx, exe, ref)
		if err != nil {
			b.Fatal(err)
		}
		if trapID != trap.NoFunction {
			b.Error(trapID)
		}
	}

	b.StopTimer()
}

func benchExecuteAr(b *testing.B, storage storage, prog []byte) {
	if storage == nil {
		b.Skip("nil")
	}

	ctx := context.Background()

	exe, mod := compileTest(benchExecutor, storage, prog, "")
	metadata := image.Metadata{MemorySizeLimit: mod.MemorySizeLimit()}
	ar, err := exe.StoreThis(ctx, "test", metadata, storage)
	exe.Close()
	if err != nil {
		panic(err)
	}
	defer ar.Close()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, trapID, err := executeArBench(ctx, storage, ar)
		if err != nil {
			b.Fatal(err)
		}
		if trapID != trap.NoFunction {
			b.Error(trapID)
		}
	}

	b.StopTimer()
}
