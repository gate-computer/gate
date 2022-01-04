// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build generate
// +build generate

package main

//go:generate go run make.go generate

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"

	"gate.computer/gate/internal/container/common"
	"gate.computer/gate/internal/librarian"
	"gate.computer/gate/internal/make/eventtypes"
	"gate.computer/gate/internal/make/runtimeassembly"
	"gate.computer/gate/internal/make/runtimeerrors"
	. "import.name/make"
)

func main() { Main(targets, "make.go", "go.mod") }

const (
	muslccVersion = "10.2.1"
	muslccURL     = "https://more.musl.cc/" + muslccVersion + "/x86_64-linux-musl/"
)

func targets() (targets Tasks) {
	var (
		PREFIX     = Getvar("PREFIX", "/usr/local")
		LIBEXECDIR = Getvar("LIBEXECDIR", Join(PREFIX, "lib/gate"))

		PROTOC = Getvar("PROTOC", "protoc")

		GO         = Getvar("GO", "go")
		GOFMT      = Getvar("GOFMT", "gofmt")
		TAGS       = Getvar("TAGS", "gateexecdir")
		buildflags = Flatten(
			"-ldflags="+Getvar("BUILDLDFLAGS", "-X gate.computer/gate/runtime/container.ExecDir="+LIBEXECDIR),
			"-tags="+TAGS,
			Fields(Getvar("BUILDFLAGS", "")),
		)

		CCACHE   = Getvar("CCACHE", LookPath("ccache"))
		CXX      = Getvar("CXX", "c++")
		CPPFLAGS = Getvar("CPPFLAGS", "-DNDEBUG")
		CXXFLAGS = Getvar("CXXFLAGS", "-O2 -Wall -Wextra -Wimplicit-fallthrough -g")
		LDFLAGS  = Getvar("LDFLAGS", "")

		WASMCXX = Getvar("WASMCXX", "clang++")
	)

	testdata := targets.Add(Target("testdata",
		testdataTask(CCACHE, WASMCXX),
	))

	library := targets.Add(Target("library",
		libraryTask(CCACHE, WASMCXX),
	))

	goSources := targets.Add(Target("go",
		protoTask(PROTOC, GO),
		eventtypes.Task(GOFMT),
		runtimeerrors.Task(GOFMT),
		runtimeassembly.Task(GO),
	))

	executor := targets.Add(Target("executor",
		goSources,
		executorTask("lib/gate", CCACHE, CXX, CPPFLAGS, CXXFLAGS, LDFLAGS),
	))
	loader := targets.Add(Target("loader",
		goSources,
		loaderTask("lib/gate", "tmp", GOARCH, CCACHE, CXX, CPPFLAGS, CXXFLAGS, LDFLAGS),
	))
	lib := targets.Add(TargetDefault("lib",
		executor,
		loader,
	))

	bin := targets.Add(TargetDefault("bin",
		goSources,
		Command(GO, "build", buildflags, "-o", "bin/gate", "./cmd/gate"),
		Command(GO, "build", buildflags, "-o", "bin/gate-daemon", "./cmd/gate-daemon"),
		Command(GO, "build", buildflags, "-o", "bin/gate-runtime", "./cmd/gate-runtime"),
		Command(GO, "build", buildflags, "-o", "bin/gate-server", "./cmd/gate-server"),
	))

	targets.Add(Target("inspect",
		loader,
		loaderInspectTask(CCACHE, CXX, CPPFLAGS, CXXFLAGS, LDFLAGS),
	))

	goTestBinaries := Group(
		goSources,
		Command(GO, "build", "-o", "tmp/test-grpc-service", "./internal/test/grpc-service"),
	)
	targets.Add(Target("check",
		goSources,
		Command(GO, "vet", "./..."),
		lib,
		goTestBinaries,
		goTestTask(GO, TAGS),
		bin,
		Env{"GOARCH": "amd64"}.Command(GO, "build", "-o", "/dev/null", "./..."),
		Env{"GOARCH": "arm64"}.Command(GO, "build", "-o", "/dev/null", "./..."),
		Env{"GOOS": "darwin"}.Command(GO, "build", "-o", "/dev/null", "./cmd/gate"),
		Env{"GOOS": "windows"}.Command(GO, "build", "-o", "/dev/null", "./cmd/gate"),
		Command(GO, "build", "-o", "/dev/null", "./cmd/gate-resource"),
	))

	targets.Add(Target("benchmark",
		lib,
		benchmarkTask(GO, TAGS),
	))

	prebuild := prebuildTask(CCACHE, CPPFLAGS, CXXFLAGS, LDFLAGS)
	targets.Add(Target("prebuild",
		prebuild,
		goTestBinaries,
		Env{"CGO_ENABLED": "0"}.Command(GO, "test", "-count=1", "./..."), // No gateexecdir tag.
	))

	targets.Add(Target("generate",
		testdata,
		library,
		goSources,
		prebuild,
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

func testdataTask(CCACHE, WASMCXX string) Task {
	var (
		WAT2WASM = Getvar("WAT2WASM", "wat2wasm")

		wasimodule  = "testdata/wasi-libc"
		wasiinclude = Join(wasimodule, "libc-bottom-half/headers/public")

		cxxflags = Flatten(
			"--target=wasm32",
			"-I"+wasiinclude,
			"-Iinclude",
			"-Os",
			"-Wall",
			"-Wextra",
			"-Wimplicit-fallthrough",
			"-Wl,--allow-undefined",
			"-Wl,--no-entry",
			"-fno-builtin",
			"-fno-exceptions",
			"-fno-inline",
			"-g",
			"-nostdlib",
			"-std=c++2a",
		)

		includes = Globber(
			"include/gate.h",
			Join(wasiinclude, "wasi/*.h"),
		)
	)

	task := func(source string, flags ...string) Task {
		target := ReplaceSuffix(source, ".wasm")

		cmd := Wrap(CCACHE, WASMCXX, cxxflags)
		if strings.HasSuffix(source, ".wat") {
			cmd = Flatten(WAT2WASM)
		}

		return If(Outdated(target, Flattener(source, includes)),
			Command(cmd, flags, "-o", target, source),
			Command("chmod", "-x", target),
		)
	}

	return Group(
		If(Missing(wasiinclude),
			Func(func() error {
				return fmt.Errorf("git submodule %s has not been checked out", wasimodule)
			}),
		),

		task("testdata/abi.cpp", "-Wl,--export-all"),
		task("testdata/hello-debug.cpp", "-Wl,--export=debug"),
		task("testdata/hello.cpp", "-Wl,--export=greet,--export=twice,--export=multi,--export=repl,--export=fail,--export=test_ext"),
		task("testdata/nop.wat"),
		task("testdata/randomseed.cpp", "-Wl,--export=dump,--export=toomuch,--export=toomuch2"),
		task("testdata/suspend.cpp", "-Wl,--export=loop,--export=loop2"),
		task("testdata/time.cpp", "-Wl,--export=check"),
	)
}

func libraryTask(CCACHE, WASMCXX string) Task {
	var (
		WASMLD      = Getvar("WASMLD", "wasm-ld")
		WASMOBJDUMP = Getvar("WASMOBJDUMP", "wasm-objdump")

		flags = Flatten(
			"--target=wasm32",
			"-Iinclude",
			"-O2",
			"-Wall",
			"-Wextra",
			"-Wimplicit-fallthrough",
			"-Wno-return-type-c-linkage",
			"-Wno-unused-parameter",
			"-Wno-unused-private-field",
			"-finline-functions",
			"-fno-exceptions",
			"-nostdlib",
			"-std=c++17",
		)

		include = "include/rt.h"
		source  = "runtime/abi/library/library.cpp"
		object  = "tmp/library.wasm"
		output  = "runtime/abi/library.go"
	)

	return If(Outdated(output, Flattener(source, include)),
		DirectoryOf(object),
		CommandWrap(CCACHE, WASMCXX, flags, "-c", "-o", object, source),
		Func(func() error {
			Println("Making", output)
			return librarian.Link(output, WASMLD, WASMOBJDUMP, "abi", false, object)
		}),
	)
}

func protoTask(PROTOC, GO string) Task {
	includes := Globber(
		"server/api/*.proto",
		"server/detail/*.proto",
	)

	var tasks Tasks

	addCompiler := func(pkg string) {
		binary := Join("tmp", Base(pkg))

		tasks.Add(If(Outdated(binary, nil),
			Command(GO, "build", "-o", binary, pkg)),
		)
	}

	addCompiler("google.golang.org/protobuf/cmd/protoc-gen-go")
	addCompiler("google.golang.org/grpc/cmd/protoc-gen-go-grpc")

	addProto := func(proto, supplement, suffix string) {
		gofile := ReplaceSuffix(proto, suffix+".pb.go")

		tasks.Add(If(Outdated(gofile, Flattener(proto, includes)),
			Command(PROTOC,
				"--plugin=tmp/protoc-gen-go"+supplement,
				"--go"+supplement+"_out=.",
				"--go"+supplement+"_opt=paths=source_relative",
				proto,
			),
		))
	}

	addProto("internal/manifest/manifest.proto", "", "")
	addProto("internal/webserverapi/webserverapi.proto", "", "")
	addProto("server/api/server.proto", "", "")
	addProto("server/detail/detail.proto", "", "")
	addProto("server/event/event.proto", "", "")
	addProto("service/grpc/api/service.proto", "", "")
	addProto("service/grpc/api/service.proto", "-grpc", "_grpc")

	return Group(tasks...)
}

func executorTask(bindir, CCACHE, CXX, CPPFLAGS, CXXFLAGS, LDFLAGS string) Task {
	var (
		cppflags = Flatten(
			"-Iruntime/include",
			`-DGATE_COMPAT_VERSION="`+common.CompatVersion+`"`,
			`-DGATE_LOADER_FILENAME="`+common.LoaderFilename+`"`,
			Fields(CPPFLAGS),
		)

		cxxflags = Flatten(
			"-fno-exceptions",
			"-std=c++17",
			Fields(CXXFLAGS),
		)

		ldflags = Flatten(
			"-static",
			Fields(LDFLAGS),
		)

		includes = Globber(
			"runtime/include/*.hpp",
		)

		source = "runtime/executor/executor.cpp"
		binary = Join(bindir, common.ExecutorFilename)
	)

	return If(Outdated(binary, Flattener(source, includes)),
		DirectoryOf(binary),
		Command(CXX, cppflags, cxxflags, ldflags, "-o", binary, source),
	)
}

func loaderTask(bindir, objdir, arch, CCACHE, CXX, CPPFLAGS, CXXFLAGS, LDFLAGS string) Task {
	var (
		cppflags = Flatten(
			"-DGATE_LOADER_ADDR=0x200000000",
			"-DPIE",
			"-I"+Join("runtime/loader", arch),
			"-I"+Join("runtime/include", arch),
			"-I"+Join("runtime/include"),
			Fields(CPPFLAGS),
		)

		cxxflags = Flatten(
			"-fPIE",
			"-fno-builtin",
			"-fno-exceptions",
			"-fno-stack-protector",
			"-std=c++17",
			Fields(CXXFLAGS),
		)

		ldflags = Flatten(
			"-Wl,--build-id=none",
			"-Wl,-Ttext-segment=0x200000000",
			"-Wl,-z,noexecstack",
			"-nostdlib",
			"-static",
			Fields(LDFLAGS),
		)

		includes = Globber(
			"runtime/include/*.hpp",
			"runtime/include/*/*.hpp",
			"runtime/loader/*.S",
			"runtime/loader/*.hpp",
			"runtime/loader/*/*.S",
			"runtime/loader/*/*.hpp",
		)

		start    = Join("runtime/loader", arch, "start.S")
		loader   = Join("runtime/loader/loader.cpp")
		runtime2 = Join("runtime/loader", arch, "runtime2.S")
		binary   = Join(bindir, common.LoaderFilename)
	)

	var objects []string
	var tasks Tasks

	addCompilation := func(source string, flags ...interface{}) {
		object := Join(objdir, ReplaceSuffix(source, ".o"))
		objects = append(objects, object)

		tasks.Add(If(Outdated(object, Flattener(source, includes)),
			DirectoryOf(object),
			CommandWrap(CCACHE, CXX, flags, "-c", "-o", object, source),
		))
	}

	addCompilation(start, cppflags)
	addCompilation(loader, cppflags, cxxflags)
	addCompilation(runtime2, cppflags)

	tasks.Add(If(Outdated(binary, Thunk(objects...)),
		DirectoryOf(binary),
		CommandWrap(CCACHE, CXX, cxxflags, ldflags, "-o", binary, objects),
	))

	return Group(tasks...)
}

func loaderInspectTask(CCACHE, CXX, CPPFLAGS, CXXFLAGS, LDFLAGS string) Task {
	var (
		PYTHON = Getvar("PYTHON", "python3")

		cppflags = Flatten(
			"-DPIE",
			"-I"+Join("runtime/include", GOARCH),
			"-I"+Join("runtime/include"),
			Fields(CPPFLAGS),
		)

		cxxflags = Flatten(
			"-fPIE",
			"-fno-exceptions",
			"-fno-stack-protector",
			"-std=c++2a",
			Fields(CXXFLAGS),
		)

		ldflags = Flatten(
			"-static",
			Fields(LDFLAGS),
		)

		includes = Globber(
			"runtime/include/*.hpp",
			"runtime/include/*/*.hpp",
		)

		start    = Join("tmp/runtime/loader", GOARCH, "start.o")
		runtime2 = Join("tmp/runtime/loader", GOARCH, "runtime2.o")

		signal = "runtime/loader/inspect/signal.cpp"
		stack  = "runtime/loader/inspect/stack.cpp"
	)

	testTask := func(run func(src, bin string) error, source, lib string, flags ...string) Task {
		object := Join("tmp", ReplaceSuffix(source, ".o"))
		binary := Join("tmp", ReplaceSuffix(source, ""))
		stamp := Join("tmp", ReplaceSuffix(source, ".stamp"))

		return If(Outdated(stamp, Flattener(source, lib, includes)),
			DirectoryOf(object),
			CommandWrap(CCACHE, CXX, cppflags, cxxflags, "-c", "-o", object, source),
			DirectoryOf(binary),
			CommandWrap(CCACHE, CXX, cxxflags, flags, ldflags, "-o", binary, lib, object),
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
			"-nostdlib",
		),
	)
}

func goTestTask(GO, TAGS string) Task {
	TEST := Getvar("TEST", "")

	TESTFLAGS := "-count=1 -race"
	if TEST != "" {
		TESTFLAGS += " -v"
	}
	TESTFLAGS = Getvar("TESTFLAGS", TESTFLAGS)

	testflags := Flatten(
		"-ldflags="+Getvar("TESTLDFLAGS", ""),
		"-run="+TEST,
		"-tags="+TAGS,
		Fields(TESTFLAGS),
	)

	return Command(GO, "test", testflags, "./...")
}

func benchmarkTask(GO, TAGS string) Task {
	var (
		PERFLOCK  = Getvar("PERFLOCK", "perflock")
		BENCHSTAT = Getvar("BENCHSTAT", "benchstat")

		BENCH      = Getvar("BENCH", ".")
		benchflags = Flatten(
			"-bench="+BENCH,
			"-tags="+TAGS,
		)
		benchcmd = Wrap(PERFLOCK, GO, "test", "-run=-", benchflags, "./...")

		BENCHSTATSNEW = Getvar("BENCHSTATSNEW", "bench-new.txt")
		BENCHSTATSOLD = Getvar("BENCHSTATSOLD", "bench-old.txt")
	)

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

		if Exists(BENCHSTATSOLD) {
			if err := Run(BENCHSTAT, BENCHSTATSOLD, BENCHSTATSNEW); err != nil {
				return err
			}
		}

		return nil
	})
}

func prebuildTask(CCACHE, CPPFLAGS, CXXFLAGS, LDFLAGS string) Task {
	var (
		CURL      = Getvar("CURL", "curl")
		SHA512SUM = Getvar("SHA512SUM", "sha512sum")
		TAR       = Getvar("TAR", "tar")
		GZIP      = Getvar("GZIP", "zopfli")
	)

	archTask := func(arch, triplet string) Task {
		var (
			muslccdir = Join("tmp", "muslcc-"+muslccVersion)
			tarname   = fmt.Sprintf("%s-cross.tgz", triplet)
			tarpath   = Join(muslccdir, tarname)
			toolchain = fmt.Sprintf("%s/%s-cross/bin/%s-", muslccdir, triplet, triplet)
			cxx       = toolchain + "c++"
			objcopy   = toolchain + "objcopy"
			strip     = toolchain + "strip"
			workdir   = Join("tmp/prebuild", arch)
		)

		packTask := func(name, fullname string) Task {
			var (
				compiled = Join(workdir, fullname)
				temp     = Join(workdir, name)
				packed   = Join("internal/container/child/binary", fmt.Sprintf("%s.%s-%s.gz", name, GOOS, arch))
			)

			return If(Outdated(packed, Thunk(compiled)),
				Command(objcopy, "-R", ".comment", "-R", ".eh_frame", "-R", ".note.gnu.property", compiled, temp),
				Command(strip, temp),
				Command(GZIP, temp),
				Installation(packed, temp+".gz", false),
			)
		}

		return Group(
			If(Missing(cxx),
				If(Missing(tarpath),
					Directory(muslccdir),
					Command(CURL, "-o", tarpath, muslccURL+tarname),
				),
				Command(SHA512SUM, "-c", fmt.Sprintf("muslcc.%s.sha512sum", arch)),
				Command(TAR, "xf", tarpath, "-C", muslccdir),
			),
			executorTask(workdir, CCACHE, cxx, CPPFLAGS, CXXFLAGS, LDFLAGS),
			loaderTask(workdir, workdir, arch, CCACHE, cxx, CPPFLAGS, CXXFLAGS, LDFLAGS),
			packTask("executor", common.ExecutorFilename),
			packTask("loader", common.LoaderFilename),
		)
	}

	return Group(
		archTask("amd64", "x86_64-linux-musl"),
		archTask("arm64", "aarch64-linux-musl"),
	)
}
