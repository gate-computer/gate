// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package run_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"testing"

	"github.com/tsavola/gate/internal/runtest"
	"github.com/tsavola/gate/run"
	"github.com/tsavola/wag/compile"
	"github.com/tsavola/wag/trap"
)

func readProgram(testName string) []byte {
	f := openProgram(testName)
	defer f.Close()

	data, err := ioutil.ReadAll(f)
	if err != nil {
		panic(err)
	}

	return data
}

const (
	benchStackSize = 65536
)

var (
	benchRT = runtest.NewRuntime(nil)

	benchProgNop   = readProgram("nop")
	benchProgHello = readProgram("hello")
	benchProgPeer  = readProgram("peer")
)

func compileBenchmark(prog []byte) (m *compile.Module) {
	m = new(compile.Module)

	err := run.Load(m, bytes.NewReader(prog), benchRT.Runtime, nil, nil, nil)
	if err != nil {
		panic(err)
	}

	return
}

func prepareBenchmark(m *compile.Module) (image *run.Image) {
	image = new(run.Image)

	if err := image.Init(context.Background(), benchRT.Runtime); err != nil {
		panic(err)
	}

	_, memorySize := m.MemoryLimits()

	if err := image.Populate(m, memorySize, benchStackSize); err != nil {
		panic(err)
	}

	return
}

func executeBenchmark(image *run.Image, output io.Writer,
) (exit int, trapId trap.Id, err error) {
	var proc run.Process

	err = proc.Init(context.Background(), benchRT.Runtime, image, nil)
	if err != nil {
		return
	}
	defer proc.Kill(benchRT.Runtime)

	exit, trapId, err = run.Run(context.Background(), benchRT.Runtime, &proc, image, &testServiceRegistry{output})
	return
}

func BenchmarkCompileNop(b *testing.B) {
	benchmarkCompile(b, benchProgNop)
}

func BenchmarkCompileHello(b *testing.B) {
	benchmarkCompile(b, benchProgHello)
}

func BenchmarkCompilePeer(b *testing.B) {
	benchmarkCompile(b, benchProgPeer)
}

func benchmarkCompile(b *testing.B, prog []byte) {
	for i := 0; i < b.N; i++ {
		compileBenchmark(prog)
	}
}

func BenchmarkPrepareNop(b *testing.B) {
	benchmarkPrepare(b, benchProgNop)
}

func BenchmarkPrepareHello(b *testing.B) {
	benchmarkPrepare(b, benchProgHello)
}

func BenchmarkPreparePeer(b *testing.B) {
	benchmarkPrepare(b, benchProgPeer)
}

func benchmarkPrepare(b *testing.B, prog []byte) {
	m := compileBenchmark(prog)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		image := prepareBenchmark(m)
		image.Release(benchRT.Runtime)
	}
}

func BenchmarkExecuteNop(b *testing.B) {
	benchmarkExecute(b, benchProgNop, "")
}

func BenchmarkExecuteHello(b *testing.B) {
	benchmarkExecute(b, benchProgHello, "hello world\n")
}

func benchmarkExecute(b *testing.B, prog []byte, expectOutput string) {
	m := compileBenchmark(prog)
	image := prepareBenchmark(m)
	defer image.Release(benchRT.Runtime)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var output bytes.Buffer

		exit, trapId, err := executeBenchmark(image, &output)
		if err != nil {
			panic(err)
		}
		if trapId != 0 {
			panic(trapId)
		}
		if exit != 0 {
			panic(exit)
		}

		if output.String() != expectOutput {
			panic(fmt.Sprint(output.Bytes()))
		}
	}
}
