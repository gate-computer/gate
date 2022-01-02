// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore
// +build ignore

package main

import (
	"io"
	"io/ioutil"
	"os"
	"os/exec"

	"gate.computer/gate/internal/container/common"
	"gate.computer/gate/internal/make/eventtypes"
	"gate.computer/gate/internal/make/runtimeassembly"
	"gate.computer/gate/internal/make/runtimeerrors"
	. "import.name/make"
)

func main() { Main(targets, "make.go", "go.mod") }

func targets() (targets Tasks) {
	ARCH := Getvar("ARCH", GOARCH)
	Setenv("GOARCH", ARCH)

	var (
		PREFIX     = Getvar("PREFIX", "/usr/local")
		LIBEXECDIR = Getvar("LIBEXECDIR", Join(PREFIX, "lib/gate"))

		PROTOC = Getvar("PROTOC", "protoc")

		GO         = Getvar("GO", "go")
		GOFMT      = Getvar("GOFMT", "gofmt")
		TAGS       = Getvar("TAGS", "gateexecdir")
		BUILDFLAGS = Flatten(
			"-ldflags="+Getvar("BUILDLDFLAGS", "-X gate.computer/gate/runtime/container.ExecDir="+LIBEXECDIR),
			"-tags="+TAGS,
			Fields(Getvar("BUILDFLAGS", "")),
		)

		CXX      = Getvar("CXX", "c++")
		CPPFLAGS = Fields(Getvar("CPPFLAGS", "-DNDEBUG"))
		CXXFLAGS = Fields(Getvar("CXXFLAGS", "-O2 -Wall -Wextra -Wimplicit-fallthrough -Wno-unused-parameter -Wno-write-strings -fomit-frame-pointer -g -std=c++17"))
		LDFLAGS  = Fields(Getvar("LDFLAGS", ""))
	)

	targets.Add(Target("library",
		Command(GO, "run", "./cmd/gate-librarian", "-v", "-go=abi", "runtime/abi/library.go", "--", "runtime/abi/library/compile.sh", "-c", "-o", "/dev/stdout")),
	)

	sources := Group(
		protoTask(PROTOC, GO, ARCH),
		eventtypes.Task(GOFMT),
		runtimeerrors.Task(GOFMT),
		runtimeassembly.Task(GO),
	)

	executor := targets.Add(Target("executor",
		sources,
		executorTask(CXX, CPPFLAGS, CXXFLAGS, LDFLAGS)),
	)
	loader := targets.Add(Target("loader",
		sources,
		loaderTask(CXX, ARCH, CPPFLAGS, CXXFLAGS, LDFLAGS)),
	)
	lib := targets.Add(TargetDefault("lib",
		executor,
		loader,
	))

	targets.Add(Target("prebuild",
		sources,
		Command("./prebuild.sh")),
	)

	bin := targets.Add(TargetDefault("bin",
		sources,
		Command(GO, "build", BUILDFLAGS, "-o", "bin/gate", "./cmd/gate"),
		Command(GO, "build", BUILDFLAGS, "-o", "bin/gate-daemon", "./cmd/gate-daemon"),
		Command(GO, "build", BUILDFLAGS, "-o", "bin/gate-runtime", "./cmd/gate-runtime"),
		Command(GO, "build", BUILDFLAGS, "-o", "bin/gate-server", "./cmd/gate-server"),
	))

	targets.Add(Target("testdata",
		Command("make", "-C", "testdata")),
	)

	targets.Add(Target("check",
		targets.Add(Target("check/loader",
			loader,
			loaderTestTask(CXX, ARCH, CPPFLAGS, CXXFLAGS, LDFLAGS),
		)),
		targets.Add(Target("check/go",
			sources,
			Command(GO, "vet", "./..."),
			lib,
			goTestTask(GO, ARCH, TAGS),
			bin,
			Env{"GOARCH": "amd64"}.Command(GO, "build", "-o", "/dev/null", "./..."),
			Env{"GOARCH": "arm64"}.Command(GO, "build", "-o", "/dev/null", "./..."),
			Env{"GOOS": "darwin"}.Command(GO, "build", "-o", "/dev/null", "./cmd/gate"),
			Env{"GOOS": "windows"}.Command(GO, "build", "-o", "/dev/null", "./cmd/gate"),
			Command(GO, "build", "-o", "/dev/null", "./cmd/gate-resource"),
		)),
	))

	targets.Add(Target("benchmark",
		lib,
		benchmarkTask(GO, TAGS),
	))

	targets.Add(TargetDefault("installer",
		Command(GO, "build",
			"-ldflags=-X main.PREFIX="+PREFIX+" -X main.LIBEXECDIR="+LIBEXECDIR,
			"-o", "bin/install",
			"./internal/make/cmd/install",
		),
	))

	targets.Add(Target("clean",
		Removal("bin", "lib", "tmp"),
	))

	return
}

func protoTask(PROTOC, GO, ARCH string) Task {
	includes := Globber(
		"server/api/*.proto",
		"server/detail/*.proto",
	)

	var tasks Tasks

	addCompiler := func(pkg string) {
		binary := Join("tmp", ARCH, Base(pkg))

		tasks.Add(If(Outdated(binary, nil),
			Command(GO, "build", "-o", binary, pkg)),
		)
	}

	addCompiler("google.golang.org/protobuf/cmd/protoc-gen-go")
	addCompiler("google.golang.org/grpc/cmd/protoc-gen-go-grpc")

	env := Env{
		"PATH": Join("tmp", ARCH) + ":" + Getenv("PATH", ""),
	}

	addProto := func(proto, plug string) {
		tasks.Add(If(Outdated(ReplaceSuffix(proto, ".pb.go"), Flattener(proto, includes)),
			env.Command(PROTOC, plug+"_out=.", plug+"_opt=paths=source_relative", proto),
		))
	}

	addProto("internal/manifest/manifest.proto", "--go")
	addProto("internal/webserverapi/webserverapi.proto", "--go")
	addProto("server/api/server.proto", "--go")
	addProto("server/detail/detail.proto", "--go")
	addProto("server/event/event.proto", "--go")
	addProto("service/grpc/api/service.proto", "--go")
	addProto("service/grpc/api/service.proto", "--go-grpc")

	return Group(tasks...)
}

func executorTask(CXX string, CPPFLAGS, CXXFLAGS, LDFLAGS []string) Task {
	CPPFLAGS = Flatten(
		"-Iruntime/include",
		`-DGATE_COMPAT_VERSION="`+common.CompatVersion+`"`,
		CPPFLAGS,
	)

	CXXFLAGS = Flatten(
		"-fno-exceptions",
		CXXFLAGS,
	)

	LDFLAGS = Flatten(
		"-static",
		LDFLAGS,
	)

	includes := Globber(
		"runtime/include/*.hpp",
		"runtime/include/*/*.hpp",
	)

	var (
		source = "runtime/executor/executor.cpp"
		binary = Join("lib/gate", common.ExecutorFilename)
	)

	return If(Outdated(binary, Flattener(source, includes)),
		DirectoryOf(binary),
		Command(CXX, CPPFLAGS, CXXFLAGS, LDFLAGS, "-o", binary, source),
	)
}

func loaderTask(CXX, ARCH string, CPPFLAGS, CXXFLAGS, LDFLAGS []string) Task {
	CPPFLAGS = Flatten(
		"-DGATE_LOADER_ADDR=0x200000000",
		"-DPIE",
		"-I"+Join("runtime/loader", ARCH),
		"-I"+Join("runtime/include", ARCH),
		"-I"+Join("runtime/include"),
		CPPFLAGS,
	)

	CXXFLAGS = Flatten(
		"-fPIE",
		"-fno-exceptions",
		"-fno-stack-protector",
		CXXFLAGS,
	)

	LDFLAGS = Flatten(
		"-Wl,--build-id=none",
		"-Wl,-Ttext-segment=0x200000000",
		"-Wl,-z,noexecstack",
		"-nostartfiles",
		"-nostdlib",
		"-static",
		LDFLAGS,
	)

	includes := Globber(
		"runtime/include/*.hpp",
		"runtime/include/*/*.hpp",
		"runtime/loader/*.S",
		"runtime/loader/*.hpp",
		"runtime/loader/*/*.S",
		"runtime/loader/*/*.hpp",
	)

	var (
		start    = Join("runtime/loader", ARCH, "start.S")
		loader   = Join("runtime/loader/loader.cpp")
		runtime2 = Join("runtime/loader", ARCH, "runtime2.S")
		binary   = Join("lib/gate", common.LoaderFilename)
	)

	var objects []string
	var tasks Tasks

	addCompilation := func(source string, flags ...interface{}) {
		object := Join("tmp", ARCH, ReplaceSuffix(source, ".o"))
		objects = append(objects, object)

		tasks.Add(If(Outdated(object, Flattener(source, includes)),
			DirectoryOf(object),
			Command(CXX, flags, "-c", "-o", object, source),
		))
	}

	addCompilation(start, CPPFLAGS)
	addCompilation(loader, CPPFLAGS, CXXFLAGS)
	addCompilation(runtime2, CPPFLAGS)

	tasks.Add(If(Outdated(binary, Thunk(objects...)),
		DirectoryOf(binary),
		Command(CXX, CXXFLAGS, LDFLAGS, "-o", binary, objects),
	))

	return Group(tasks...)
}

func loaderTestTask(CXX, ARCH string, CPPFLAGS, CXXFLAGS, LDFLAGS []string) Task {
	PYTHON := Getvar("PYTHON", "python3")

	CPPFLAGS = Flatten(
		"-DPIE",
		"-I"+Join("runtime/include", ARCH),
		"-I"+Join("runtime/include"),
		CPPFLAGS,
	)

	CXXFLAGS = Flatten(
		"-fPIE",
		"-fno-exceptions",
		"-fno-stack-protector",
		CXXFLAGS,
	)

	LDFLAGS = Flatten(
		"-static",
		LDFLAGS,
	)

	includes := Globber(
		"runtime/include/*.hpp",
		"runtime/include/*/*.hpp",
	)

	var (
		start    = Join("tmp", ARCH, "runtime/loader", ARCH, "start.o")
		runtime2 = Join("tmp", ARCH, "runtime/loader", ARCH, "runtime2.o")

		signal = "runtime/loader/test/signal.cpp"
		stack  = "runtime/loader/test/stack.cpp"
	)

	testTask := func(run func(source, binary string) error, source, lib string, flags ...string) Task {
		object := Join("tmp", ARCH, ReplaceSuffix(source, ".o"))
		binary := Join("tmp", ARCH, ReplaceSuffix(source, ""))
		stamp := Join("tmp/stamp", ReplaceSuffix(source, ""))

		return If(Outdated(stamp, Flattener(source, lib, includes)),
			DirectoryOf(object),
			Command(CXX, CPPFLAGS, CXXFLAGS, "-c", "-o", object, source),
			DirectoryOf(binary),
			Command(CXX, CXXFLAGS, flags, LDFLAGS, "-o", binary, lib, object),
			Func(func() error {
				if err := run(source, binary); err != nil {
					return err
				}
				return Touch(stamp)
			}),
		)
	}

	runBinary := func(source, binary string) error {
		return Run(binary)
	}
	runPython := func(source, binary string) error {
		return Run(PYTHON, ReplaceSuffix(source, ".py"), binary)
	}

	return Group(
		testTask(runBinary, signal, runtime2,
			"-Wl,-Ttext-segment=0x40000000",
			"-Wl,--section-start=.runtime=0x50000000",
		),
		testTask(runPython, stack, start,
			"-nostartfiles",
			"-nostdlib",
		),
	)
}

func goTestTask(GO, ARCH, TAGS string) Task {
	TEST := Getvar("TEST", "")

	defaultTestFlags := "-count=1 -race"
	if TEST != "" {
		defaultTestFlags += " -v"
	}

	TESTFLAGS := Flatten(
		"-ldflags="+Getvar("TESTLDFLAGS", ""),
		"-run="+TEST,
		"-tags="+TAGS,
		Fields(Getvar("TESTFLAGS", defaultTestFlags)),
	)

	return Group(
		Command(GO, "build", "-o", Join("tmp", ARCH, "test-grpc-service"), "./internal/test/grpc-service"),
		Command(GO, "test", TESTFLAGS, "./..."),
	)
}

func benchmarkTask(GO, TAGS string) Task {
	var (
		PERFLOCK  = Getvar("PERFLOCK", "perflock")
		BENCHSTAT = Getvar("BENCHSTAT", "benchstat")

		BENCH      = Getvar("BENCH", ".")
		BENCHFLAGS = Flatten(
			"-bench="+BENCH,
			"-tags="+TAGS,
		)

		BENCHSTATSNEW = Getvar("BENCHSTATSNEW", "bench-new.txt")
		BENCHSTATSOLD = Getvar("BENCHSTATSOLD", "bench-old.txt")
	)

	benchcmd := Flatten(GO, "test", "-run=-", BENCHFLAGS, "./...")
	if PERFLOCK != "" {
		benchcmd = Flatten(PERFLOCK, benchcmd)
	}

	statcmd := Flatten(BENCHSTAT, BENCHSTATSOLD, BENCHSTATSNEW)

	return Func(func() error {
		Println("Running", benchcmd)

		cmd := exec.Command(benchcmd[0], benchcmd[1:]...)
		cmd.Stderr = os.Stderr

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return err
		}
		defer stdout.Close()

		result := make(chan error, 1)
		go func() { result <- cmd.Run() }()

		output, err := ioutil.ReadAll(io.TeeReader(stdout, os.Stdout))
		if err != nil {
			return err
		}

		if err := <-result; err != nil {
			return err
		}

		Println("Writing", BENCHSTATSNEW)

		if err := ioutil.WriteFile(BENCHSTATSNEW, output, 0666); err != nil {
			return err
		}

		_, err = os.Stat(BENCHSTATSOLD)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil {
			if err := Run(statcmd...); err != nil {
				return err
			}
		}

		return nil
	})
}
