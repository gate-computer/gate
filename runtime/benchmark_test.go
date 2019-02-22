// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime_test

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/tsavola/gate/image"
	"github.com/tsavola/gate/internal/test/runtimeutil"
	"github.com/tsavola/gate/runtime"
	"github.com/tsavola/wag/object"
	"github.com/tsavola/wag/object/abi"
	"github.com/tsavola/wag/trap"
)

var benchExecutor = runtimeutil.NewExecutor(context.Background(), &runtime.Config{
	LibDir: "../lib/gate/runtime",
})

var benchRegistry = &testServiceRegistry{new(bytes.Buffer)}

var (
	benchProgGainRelease = runtimeutil.MustReadFile("../../gain/target/wasm32-unknown-unknown/release/examples/hello.wasm")
	benchProgGainDebug   = runtimeutil.MustReadFile("../../gain/target/wasm32-unknown-unknown/debug/examples/hello.wasm")
)

var benchFS interface{}

func init() {
	var config image.FilesystemConfig
	if config.Path = os.Getenv("GATE_BENCH_IMAGE_FS"); config.Path != "" {
		config.Reflink = os.Getenv("GATE_BENCH_IMAGE_REFLINK") != ""
		benchFS = image.NewFilesystem(config)
	}
}

func execRefBench(ctx context.Context, exe *image.Executable, ref image.ExecutableRef,
) (exit int, trapID trap.ID, err error) {
	proc, err := runtime.NewProcess(ctx, benchExecutor.Executor, ref, nil)
	if err != nil {
		return
	}
	defer proc.Kill()

	err = proc.Start(exe)
	if err != nil {
		return
	}

	return proc.Serve(ctx, benchRegistry)
}

func execArcBench(ctx context.Context, exeBack image.BackingStore, arc image.LocalArchive,
) (exit int, trapID trap.ID, err error) {
	ref, err := image.NewExecutableRef(exeBack)
	if err != nil {
		return
	}
	defer ref.Close()

	exe, err := image.NewExecutable(exeBack, ref, arc, testStackSize, 0, 0)
	if err != nil {
		return
	}
	defer exe.Close()

	exe.Man.InitRoutine = abi.TextAddrNoFunction

	proc, err := runtime.NewProcess(ctx, benchExecutor.Executor, ref, nil)
	if err != nil {
		return
	}
	defer proc.Kill()

	err = proc.Start(exe)
	if err != nil {
		return
	}

	return proc.Serve(ctx, benchRegistry)
}

func BenchmarkMemBuildNop(b *testing.B)         { benchBuild(b, image.Memory, testProgNop) }
func BenchmarkMemBuildHello(b *testing.B)       { benchBuild(b, image.Memory, testProgHello) }
func BenchmarkMemBuildGainRelease(b *testing.B) { benchBuild(b, image.Memory, benchProgGainRelease) }
func BenchmarkMemBuildGainDebug(b *testing.B)   { benchBuild(b, image.Memory, benchProgGainDebug) }
func BenchmarkMemExecNopRef(b *testing.B)       { benchExecRef(b, image.Memory, testProgNop) }
func BenchmarkMemExecNopArc(b *testing.B)       { benchExecArc(b, image.Memory, testProgNop) }
func BenchmarkMemExecHelloArc(b *testing.B)     { benchExecArc(b, image.Memory, testProgHello) }
func BenchmarkMemExecGainDebugArc(b *testing.B) { benchExecArc(b, image.Memory, benchProgGainDebug) }

func BenchmarkFSBuildNop(b *testing.B)         { benchBuild(b, benchFS, testProgNop) }
func BenchmarkFSBuildHello(b *testing.B)       { benchBuild(b, benchFS, testProgHello) }
func BenchmarkFSBuildGainRelease(b *testing.B) { benchBuild(b, benchFS, benchProgGainRelease) }
func BenchmarkFSBuildGainDebug(b *testing.B)   { benchBuild(b, benchFS, benchProgGainDebug) }
func BenchmarkFSExecNopRef(b *testing.B)       { benchExecRef(b, benchFS, testProgNop) }
func BenchmarkFSExecNopArc(b *testing.B)       { benchExecArc(b, benchFS, testProgNop) }
func BenchmarkFSExecHelloArc(b *testing.B)     { benchExecArc(b, benchFS, testProgHello) }
func BenchmarkFSExecGainDebugArc(b *testing.B) { benchExecArc(b, benchFS, benchProgGainDebug) }

func benchBuild(b *testing.B, back interface{}, prog []byte) {
	if back == nil {
		b.Skip("nil")
	}

	arcBack := back.(image.LocalStorage)
	exeBack := back.(image.BackingStore)

	var codeMap object.CallMap

	for i := 0; i < b.N; i++ {
		codeMap.FuncAddrs = codeMap.FuncAddrs[:0]
		codeMap.CallSites = codeMap.CallSites[:0]

		arc, exe, _ := buildTest(benchExecutor, b.Name(), arcBack, exeBack, &codeMap, &codeMap, bytes.NewReader(prog), len(prog), "")
		arc.Close()
		exe.Close()
	}
}

func benchExecRef(b *testing.B, back interface{}, prog []byte) {
	if back == nil {
		b.Skip("nil")
	}

	ctx := context.Background()

	arcBack := back.(image.LocalStorage)
	exeBack := back.(image.BackingStore)

	var codeMap object.CallMap

	arc, exe, _ := buildTest(benchExecutor, b.Name(), arcBack, exeBack, &codeMap, &codeMap, bytes.NewReader(prog), len(prog), "")
	arc.Close()
	defer exe.Close()

	ref := exe.Ref()
	defer ref.Close()

	exe.Man.InitRoutine = abi.TextAddrNoFunction

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, trapID, err := execRefBench(ctx, exe, ref)
		if err != nil {
			b.Fatal(err)
		}
		if trapID != trap.NoFunction {
			b.Error(trapID)
		}
	}

	b.StopTimer()
}

func benchExecArc(b *testing.B, back interface{}, prog []byte) {
	if back == nil {
		b.Skip("nil")
	}

	ctx := context.Background()

	arcBack := back.(image.LocalStorage)
	exeBack := back.(image.BackingStore)

	var codeMap object.CallMap

	arc, exe, _ := buildTest(benchExecutor, b.Name(), arcBack, exeBack, &codeMap, &codeMap, bytes.NewReader(prog), len(prog), "")
	defer arc.Close()
	exe.Close()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, trapID, err := execArcBench(ctx, exeBack, arc)
		if err != nil {
			b.Fatal(err)
		}
		if trapID != trap.NoFunction {
			b.Error(trapID)
		}
	}

	b.StopTimer()
}
