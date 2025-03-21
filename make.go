// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build generate

package main

//go:generate go run make.go generate

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"gate.computer/internal/container/common"
	"gate.computer/internal/executable"
	"gate.computer/internal/librarian"
	"gate.computer/internal/make/runtimeerrors"
	. "import.name/make"
)

func main() { Main(targets, "make.go", "buf.gen.yaml") }

var goPackages = []string{
	"./cmd/...",
	"./gate/...",
	"./internal/...",
	"./localhost/...",
	"./shell/...",
}

func targets() (targets Tasks) {
	var (
		O = Getvar("O", "")

		ARCH = Getvar("ARCH", GOARCH)
		OS   = Getvar("OS", GOOS)

		PREFIX     = Getvar("PREFIX", "/usr/local")
		LIBEXECDIR = Getvar("LIBEXECDIR", Join(PREFIX, "lib/gate"))

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

		CHROOTPREFIX = Getvar("CHROOTPREFIX", "../gate-prebuild")
		CHROOTAMD64  = Getvar("CHROOTAMD64", Join(CHROOTPREFIX, "x86_64"))
		CHROOTARM64  = Getvar("CHROOTARM64", Join(CHROOTPREFIX, "aarch64"))
	)

	testdata := targets.Add(Target("testdata",
		testdataTask(CCACHE, WASMCXX),
	))

	library := targets.Add(Target("library",
		libraryTask(O, CCACHE, WASMCXX),
	))

	sources := Group(
		protoTask(O, GO),
		eventTypesTask(GO, GOFMT),
		runtimeerrors.Task(GOFMT),
	)

	var ccache []string
	if CCACHE != "" {
		ccache = []string{CCACHE}
	}

	executor := targets.Add(Target("executor",
		sources,
		executorTask(ccache, Join(O, "lib", ARCH, "gate"), CXX, CPPFLAGS, CXXFLAGS, LDFLAGS),
	))
	loader := targets.Add(Target("loader",
		sources,
		loaderTask(ccache, Join(O, "lib", ARCH, "gate"), Join(O, "obj", ARCH), ARCH, OS, GO, CXX, CPPFLAGS, CXXFLAGS, LDFLAGS),
	))
	lib := targets.Add(TargetDefault("lib",
		executor,
		loader,
	))

	bin := targets.Add(TargetDefault("bin",
		sources,
		binTask(O, ARCH, OS, GO, buildflags),
	))

	if ARCH == GOARCH && OS == GOOS {
		targets.Add(Target("inspect",
			loader,
			loaderInspectTask(O, CCACHE, CXX, CPPFLAGS, CXXFLAGS, LDFLAGS),
		))

		goTestBinaries := Group(
			sources,
			Command(GO, "build", "-o", Join(O, "lib", ARCH, "test-grpc-service"), "./grpc/internal/test/grpc-service"),
		)
		targets.Add(Target("check",
			sources,
			Command(GO, "vet", goPackages),
			lib,
			goTestBinaries,
			goTestTask(GO, TAGS),
			bin,
			Env{"GOARCH": "amd64"}.Command(GO, "build", "-o", "/dev/null", goPackages),
			Env{"GOARCH": "arm64"}.Command(GO, "build", "-o", "/dev/null", goPackages),
			Env{"GOOS": "darwin"}.Command(GO, "build", "-o", "/dev/null", "./cmd/gate"),
			Env{"GOOS": "windows"}.Command(GO, "build", "-o", "/dev/null", "./cmd/gate"),
			Command(GO, "build", "-o", "/dev/null", "./cmd/gate-resource"),
		))

		targets.Add(Target("benchmark",
			lib,
			benchmarkTask(O, GO, TAGS),
		))

		targets.Add(Target("generate",
			testdata,
			library,
			sources,
		))

		prebuildChrootAMD64 := targets.Add(Target("prebuild-chroot-amd64",
			prebuildChrootTask(CHROOTAMD64, "x86_64"),
		))
		prebuildChrootARM64 := targets.Add(Target("prebuild-chroot-arm64",
			prebuildChrootTask(CHROOTARM64, "aarch64"),
		))
		targets.Add(Target("prebuild-chroot",
			prebuildChrootAMD64,
			prebuildChrootARM64,
		))

		targets.Add(Target("prebuild",
			prebuildTask(O, GO, CPPFLAGS, CXXFLAGS, LDFLAGS, CHROOTAMD64, CHROOTARM64),
			goTestBinaries,
			Command(GO, "test", "-count=1", goPackages), // No gateexecdir tag.
		))
	}

	targets.Add(TargetDefault("installer",
		Command(GO, "build",
			"-ldflags=-X main.PREFIX="+PREFIX+" -X main.LIBEXECDIR="+LIBEXECDIR,
			"-o", Join(O, "bin/install"),
			"./internal/make/cmd/install",
		),
	))

	targets.Add(Target("clean",
		Removal(
			Join(O, "bin"),
			Join(O, "lib"),
			Join(O, "obj"),
		),
	))

	return
}

func testdataTask(CCACHE, WASMCXX string) Task {
	var (
		WAT2WASM = Getvar("WAT2WASM", "wat2wasm")

		wasimodule  = "testdata/wasi-libc"
		wasiinclude = Join(wasimodule, "libc-bottom-half/headers/public")
		deps        = Globber(
			"gate/include/*.h",
			"testdata/*.hpp",
			Join(wasiinclude, "wasi/*"),
		)

		cxxflags = Flatten(
			"--target=wasm32",
			"-I"+wasiinclude,
			"-Igate/include",
			"-Os",
			"-Wall",
			"-Wextra",
			"-Wimplicit-fallthrough",
			"-Wno-missing-field-initializers",
			"-Wno-vla-cxx-extension",
			"-Wl,--allow-undefined",
			"-Wl,--no-entry",
			"-fno-builtin",
			"-fno-exceptions",
			"-fno-inline",
			"-g",
			"-nostdlib",
			"-std=c++20",
		)
	)

	program := func(source string, flags ...string) Task {
		binary := ReplaceSuffix(source, ".wasm")

		cmd := Wrap(CCACHE, WASMCXX, cxxflags)
		if strings.HasSuffix(source, ".wat") {
			cmd = Flatten(WAT2WASM)
		}

		return If(Outdated(binary, Flattener(source, deps)),
			Command(cmd, flags, "-o", binary, source),
			Command("chmod", "-x", binary),
		)
	}

	return Group(
		If(Missing(wasiinclude),
			Func(func() error {
				return fmt.Errorf("git submodule %s has not been checked out", wasimodule)
			}),
		),

		program("testdata/abi.cpp", "-Wl,--export-all"),
		program("testdata/hello-debug.cpp", "-Wl,--export=debug"),
		program("testdata/hello.cpp", "-Wl,--export=greet,--export=twice,--export=multi,--export=repl,--export=fail,--export=test_ext"),
		program("testdata/nop.wat"),
		program("testdata/randomseed.cpp", "-Wl,--export=dump,--export=toomuch,--export=toomuch2"),
		program("testdata/suspend.cpp", "-Wl,--export=loop,--export=loop2"),
		program("testdata/time.cpp", "-Wl,--export=check"),
	)
}

func libraryTask(O, CCACHE, WASMCXX string) Task {
	var (
		WASMLD      = Getvar("WASMLD", "wasm-ld")
		WASMOBJDUMP = Getvar("WASMOBJDUMP", "wasm-objdump")

		deps = Globber(
			"gate/include/*.h",
			"gate/runtime/abi/library/*.cpp",
			"gate/runtime/abi/library/*.hpp",
			"internal/librarian/*.go",
		)

		flags = Flatten(
			"--target=wasm32",
			"-Igate/include",
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

		source  = "gate/runtime/abi/library/library.cpp"
		object  = Join(O, "obj", ReplaceSuffix(source, ".wasm"))
		genGo   = "gate/runtime/abi/library.gen.go"
		genWASM = "gate/runtime/abi/library.wasm"
	)

	return If(Any(Outdated(genGo, deps), Outdated(genWASM, deps)),
		DirectoryOf(object),
		CommandWrap(CCACHE, WASMCXX, flags, "-c", "-o", object, source),
		Func(func() error {
			Println("Making", genWASM)
			return librarian.Link(genWASM, WASMLD, WASMOBJDUMP, genGo, "abi", object)
		}),
	)
}

func protoTask(O, GO string) Task {
	protos := Globber(
		"gate/pb/*/*.proto",
		"gate/pb/*/*/*.proto",
		"grpc/pb/*.proto",
		"internal/pb/*/*.proto",
	)

	tasks := Tasks{
		Command(GO, "build", "-o", "lib/", "google.golang.org/protobuf/cmd/protoc-gen-go"),
		Command(GO, "build", "-o", "lib/", "google.golang.org/grpc/cmd/protoc-gen-go-grpc"),
	}

	for _, proto := range protos() {
		tasks.Add(If(Outdated(ReplaceSuffix(proto, ".pb.go"), protos),
			Command(GO, "run", "github.com/bufbuild/buf/cmd/buf", "generate", "--path", proto),
		))
	}

	return Group(tasks...)
}

func eventTypesTask(GO, GOFMT string) Task {
	var (
		deps = Globber(
			"gate/pb/server/event/*.go",
			"gate/server/event/*.go",
		)

		output = "gate/server/event/event.gen.go"
	)

	return If(Outdated(output, deps),
		Command(GO, "run", "gate/server/event/gen.go", GOFMT, output),
	)
}

func executorTask(cxxWrap []string, bindir, CXX, CPPFLAGS, CXXFLAGS, LDFLAGS string) Task {
	var (
		deps = Globber(
			"gate/runtime/executor/*.cpp",
			"gate/runtime/executor/*.hpp",
			"gate/runtime/include/*.hpp",
		)

		cppflags = Flatten(
			"-Igate/runtime/include",
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

		binary = Join(bindir, common.ExecutorFilename)
	)

	return If(Outdated(binary, deps),
		DirectoryOf(binary),
		Command(cxxWrap, CXX, cppflags, cxxflags, ldflags, "-o", binary, "gate/runtime/executor/executor.cpp"),
	)
}

func loaderTask(cxxWrap []string, bindir, objdir, arch, OS, GO, CXX, CPPFLAGS, CXXFLAGS, LDFLAGS string) Task {
	var (
		deps = Globber(
			"gate/runtime/include/*.hpp",
			"gate/runtime/include/*/*.hpp",
			"gate/runtime/loader/*.S",
			"gate/runtime/loader/*.cpp",
			"gate/runtime/loader/*.go",
			"gate/runtime/loader/*.hpp",
			"gate/runtime/loader/*/*.S",
			"gate/runtime/loader/*/*.hpp",
			"internal/error/runtime/*.go",
		)

		cppflags = Flatten(
			"-DGATE_STACK_LIMIT_OFFSET="+strconv.Itoa(executable.StackLimitOffset),
			"-DPIE",
			"-I"+Join(objdir, "gate/runtime/loader"),
			"-I"+Join("gate/runtime/loader", arch),
			"-I"+Join("gate/runtime/loader"),
			"-I"+Join("gate/runtime/include", arch),
			"-I"+Join("gate/runtime/include"),
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
			"-Wl,-Ttext-segment="+fmt.Sprintf("%#x", executable.LoaderTextAddr),
			"-Wl,-z,noexecstack",
			"-nostdlib",
			"-static",
			Fields(LDFLAGS),
		)

		binary = Join(bindir, common.LoaderFilename)
	)

	var objects []string
	var tasks Tasks

	addGen := func(filename string) {
		gen := Join(objdir, "gate/runtime/loader", filename)

		tasks.Add(DirectoryOf(gen))
		tasks.Add(Command(GO, "run", "gate/runtime/loader/gen.go", gen, arch, OS))
	}

	addGen("rt.gen.S")
	addGen("start.gen.S")

	addCompilation := func(source string, flags ...any) {
		object := Join(objdir, ReplaceSuffix(source, ".o"))
		objects = append(objects, object)

		tasks.Add(DirectoryOf(object))
		tasks.Add(Command(cxxWrap, CXX, flags, "-c", "-o", object, source))
	}

	addCompilation(Join("gate/runtime/loader", arch, "start.S"), cppflags)
	addCompilation(Join("gate/runtime/loader/loader.cpp"), cppflags, cxxflags)
	addCompilation(Join("gate/runtime/loader", arch, "rt.S"), cppflags) // Link as last.

	tasks.Add(DirectoryOf(binary))
	tasks.Add(Command(cxxWrap, CXX, cxxflags, ldflags, "-o", binary, objects))

	return If(Outdated(binary, deps), tasks...)
}

func loaderInspectTask(O, CCACHE, CXX, CPPFLAGS, CXXFLAGS, LDFLAGS string) Task {
	var (
		PYTHON = Getvar("PYTHON", "python3")

		deps = Globber(
			"gate/runtime/include/*.hpp",
			"gate/runtime/include/*/*.hpp",
			"gate/runtime/loader/inspect/*.cpp",
			"gate/runtime/loader/inspect/*.hpp",
		)

		cppflags = Flatten(
			"-DPIE",
			"-I"+Join("gate/runtime/include", GOARCH),
			"-I"+Join("gate/runtime/include"),
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

		rt    = Join(O, "obj", GOARCH, "gate/runtime/loader", GOARCH, "rt.o")
		start = Join(O, "obj", GOARCH, "gate/runtime/loader", GOARCH, "start.o")
	)

	inspection := func(run func(src, bin string) error, source, lib string, flags ...string) Task {
		object := Join(O, "obj", GOARCH, ReplaceSuffix(source, ".o"))
		binary := Join(O, "lib", GOARCH, Base(ReplaceSuffix(source, "")))

		return Group(
			If(Outdated(binary, Flattener(lib, deps)),
				DirectoryOf(object),
				CommandWrap(CCACHE, CXX, cppflags, cxxflags, "-c", "-o", object, source),
				DirectoryOf(binary),
				CommandWrap(CCACHE, CXX, cxxflags, flags, ldflags, "-o", binary, lib, object),
			),
			Func(func() error {
				return run(source, binary)
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
		inspection(runBinary, "gate/runtime/loader/inspect/signal.cpp", rt, "-Wl,-Ttext-segment=0x40000000"),
		inspection(runPython, "gate/runtime/loader/inspect/stack.cpp", start, "-nostdlib"),
	)
}

func binTask(O, ARCH, OS, GO string, flags []string) Task {
	env := Env{}
	if ARCH != GOARCH {
		env["GOARCH"] = ARCH
	}
	if OS != GOOS {
		env["GOOS"] = OS
	}

	return Group(
		env.Command(GO, "build", flags, "-o", Join(O, "bin/gate"), "./cmd/gate"),
		env.Command(GO, "build", flags, "-o", Join(O, "bin/gate-daemon"), "./cmd/gate-daemon"),
		env.Command(GO, "build", flags, "-o", Join(O, "bin/gate-runtime"), "./cmd/gate-runtime"),
		env.Command(GO, "build", flags, "-o", Join(O, "bin/gate-server"), "./cmd/gate-server"),
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

	return Command(GO, "test", testflags, goPackages)
}

func benchmarkTask(O, GO, TAGS string) Task {
	var (
		PERFLOCK  = Getvar("PERFLOCK", "perflock")
		BENCHSTAT = Getvar("BENCHSTAT", "benchstat")

		BENCH      = Getvar("BENCH", ".")
		benchflags = Flatten(
			"-bench="+BENCH,
			"-tags="+TAGS,
		)
		benchcmd = Wrap(PERFLOCK, GO, "test", "-run=-", benchflags, goPackages)

		BENCHSTATSNEW = Getvar("BENCHSTATSNEW", Join(O, "bench-new.txt"))
		BENCHSTATSOLD = Getvar("BENCHSTATSOLD", Join(O, "bench-old.txt"))
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

		if err := ioutil.WriteFile(BENCHSTATSNEW, output, 0o666); err != nil {
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

func prebuildChrootTask(chroot, arch string) Task {
	var (
		scriptdir = "alpine-chroot-install"
		script    = Join(scriptdir, "alpine-chroot-install")
	)

	chroot, err := filepath.Abs(chroot)
	if err != nil {
		panic(err)
	}

	srcdir, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	return Group(
		If(Missing(script),
			Func(func() error {
				return fmt.Errorf("git submodule %s has not been checked out", scriptdir)
			}),
		),

		Command("sudo", script,
			"-a", arch,
			"-d", chroot,
			"-i", srcdir,
			"-p", "build-base",
			"-p", "g++",
			"-p", "linux-headers",
			"-p", "zopfli",
		),
	)
}

func prebuildTask(O, GO, CPPFLAGS, CXXFLAGS, LDFLAGS, CHROOTAMD64, CHROOTARM64 string) Task {
	u, err := user.Current()
	if err != nil {
		panic(err)
	}

	archTask := func(arch, chroot string) Task {
		var (
			wrap   = []string{Join(chroot, "enter-chroot"), "-u", u.Username}
			objdir = Join(O, "obj", arch, "prebuild")
		)

		packTask := func(name, fullname string) Task {
			var (
				compiled = Join(objdir, fullname)
				temp     = Join(objdir, name)
				packed   = Join("internal/container/child/binary", fmt.Sprintf("%s.linux-%s.gz", name, arch))
			)

			return If(Outdated(packed, Thunk(compiled)),
				Command(wrap, "objcopy", "-R", ".comment", "-R", ".eh_frame", "-R", ".note.gnu.property", compiled, temp),
				Command(wrap, "strip", temp),
				Command(wrap, "zopfli", temp),
				Installation(packed, temp+".gz", false),
			)
		}

		return Group(
			executorTask(wrap, objdir, "c++", CPPFLAGS, CXXFLAGS, LDFLAGS),
			loaderTask(wrap, objdir, objdir, arch, GOOS, GO, "c++", CPPFLAGS, CXXFLAGS, LDFLAGS),
			packTask("executor", common.ExecutorFilename),
			packTask("loader", common.LoaderFilename),
		)
	}

	return Group(
		archTask("amd64", CHROOTAMD64),
		archTask("arm64", CHROOTARM64),
	)
}
