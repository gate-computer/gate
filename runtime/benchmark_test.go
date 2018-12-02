// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/tsavola/gate/image"
	"github.com/tsavola/gate/internal/test/runtimeutil"
	"github.com/tsavola/gate/runtime"
	"github.com/tsavola/wag/object/abi"
	"github.com/tsavola/wag/object/stack"
	"github.com/tsavola/wag/trap"
)

const noFunction = runtime.InitRoutine(abi.TextAddrNoFunction)

var benchExecutor = runtimeutil.NewExecutor(context.Background(), &runtime.Config{
	LibDir: "../lib/gate/runtime",
})

var benchRegistry = &testServiceRegistry{new(bytes.Buffer)}

var (
	benchProgGainRelease = runtimeutil.MustReadFile("../../gain/target/wasm32-unknown-unknown/release/examples/hello.wasm")
	benchProgGainDebug   = runtimeutil.MustReadFile("../../gain/target/wasm32-unknown-unknown/debug/examples/hello.wasm")
)

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

func executeArBench(ctx context.Context, ar image.Archive) (exit int, trapID trap.ID, err error) {
	ref, err := image.NewExecutableRef(image.Memory)
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

func BenchmarkCompileNop(b *testing.B)         { benchCompile(b, testProgNop) }
func BenchmarkCompileHello(b *testing.B)       { benchCompile(b, testProgHello) }
func BenchmarkCompileGainRelease(b *testing.B) { benchCompile(b, benchProgGainRelease) }
func BenchmarkCompileGainDebug(b *testing.B)   { benchCompile(b, benchProgGainDebug) }
func BenchmarkExecuteNopRef(b *testing.B)      { benchExecuteRef(b, testProgNop) }
func BenchmarkExecuteNopAr(b *testing.B)       { benchExecuteAr(b, testProgNop) }
func BenchmarkExecuteHelloAr(b *testing.B)     { benchExecuteAr(b, testProgHello) }
func BenchmarkExecuteGainDebugAr(b *testing.B) { benchExecuteAr(b, benchProgGainDebug) }

func benchCompile(b *testing.B, prog []byte) {
	for i := 0; i < b.N; i++ {
		exe, _ := compileTest(benchExecutor, prog, "")
		exe.Close()
	}
}

func benchExecuteRef(b *testing.B, prog []byte) {
	ctx := context.Background()

	exe, _ := compileTest(benchExecutor, prog, "")
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

func benchExecuteAr(b *testing.B, prog []byte) {
	ctx := context.Background()

	exe, mod := compileTest(benchExecutor, prog, "")
	metadata := &image.Metadata{MemorySizeLimit: mod.MemorySizeLimit()}
	ar, err := exe.StoreThis(ctx, "test", metadata, image.Memory)
	exe.Close()
	if err != nil {
		panic(err)
	}
	defer ar.Close()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, trapID, err := executeArBench(ctx, ar)
		if err != nil {
			b.Fatal(err)
		}
		if trapID != trap.NoFunction {
			b.Error(trapID)
		}
	}

	b.StopTimer()
}
