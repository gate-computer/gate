// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore
// +build ignore

package main

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	goruntime "runtime"

	"gate.computer/gate/internal/make/runtime"
	. "import.name/make"
)

func main() { Main(targets) }

const (
	S   = "S"
	cpp = "cpp"
	o   = "o"
)

const GATE_COMPAT_VERSION = "0.0" // TODO
const GATE_COMPAT_MAJOR = "0"     // TODO

var arch = map[string]string{
	"amd64": "x86_64",
	"arm64": "aarch64",
}[goruntime.GOARCH]

func targets() (top Targets) {
	var (
		GO  = Option("GO", "go")
		CXX = Option("CXX", "c++")

		PREFIX     = Option("PREFIX", "/usr/local")
		LIBEXECDIR = Option("LIBEXECDIR", PREFIX+"/lib/gate")
	)

	TAGS := Option("TAGS", "gateexecdir")

	BUILDLDFLAGS := "-X gate.computer/gate/runtime/container.ExecDir=" + LIBEXECDIR
	BUILDFLAGS := []string{
		"-ldflags=" + BUILDLDFLAGS,
		"-tags=" + TAGS,
	}

	TESTLDFLAGS := ""
	TESTFLAGS := []string{
		"-count=1",
		"-ldflags=" + TESTLDFLAGS,
		"-race",
		"-tags=" + TAGS,
	}

	TEST := Option("TEST", "")
	if TEST != "" {
		TESTFLAGS = append(TESTFLAGS, "-run="+TEST)
		TESTFLAGS = append(TESTFLAGS, "-v")
	}

	executor := top.Target("executor",
		executorTask(CXX),
	)

	loader := top.Target("loader",
		runtime.Task(),
		loaderTask(CXX),
	)

	lib := top.TargetDefault("lib", executor, loader)

	bin := top.TargetDefault("bin",
		Command(GO, "build", BUILDFLAGS, "-o", "bin/gate", "./cmd/gate"),
		Command(GO, "build", BUILDFLAGS, "-o", "bin/gate-daemon", "./cmd/gate-daemon"),
		Command(GO, "build", BUILDFLAGS, "-o", "bin/gate-runtime", "./cmd/gate-runtime"),
		Command(GO, "build", BUILDFLAGS, "-o", "bin/gate-server", "./cmd/gate-server"),
	)

	top.Target("check",
		lib,
		Command(GO, "build", "-o", "/dev/null", "./cmd/gate-librarian"),
		Command(GO, "build", "-o", "/dev/null", "./cmd/gate-resource"),
		System("make -C runtime/loader/test check"),
		Env{"GOARCH": "amd64"}.Command(GO, "build", "-o", "/dev/null", "./..."),
		Env{"GOARCH": "arm64"}.Command(GO, "build", "-o", "/dev/null", "./..."),
		Env{"GOOS": "darwin"}.Command(GO, "build", "-o", "/dev/null", "./cmd/gate"),
		Env{"GOOS": "windows"}.Command(GO, "build", "-o", "/dev/null", "./cmd/gate"),
		Command(GO, "vet", "./..."),
		Command(GO, "build", BUILDFLAGS, "-o", "tmp/bin/test-grpc-service", "./internal/test/grpc-service"),
		Command(GO, "test", TESTFLAGS, "./..."),
		bin,
	)

	top.Target("benchmark",
		lib,
		benchmarkTask(GO, TAGS),
	)

	top.Target("clean",
		Func(func() error { return os.RemoveAll("bin") }),
		Func(func() error { return os.RemoveAll("lib") }),
		Func(func() error { return os.RemoveAll("tmp") }),
		Command("make", "-C", "runtime/executor", "clean"),
		Command("make", "-C", "runtime/loader", "clean"),
		Command("make", "-C", "runtime/loader/test", "clean"),
	)

	return
}

func executorTask(CXX string) Task {
	var (
		CPPFLAGS = []string{"-Iruntime/include", "-DNDEBUG", `-DGATE_COMPAT_VERSION="` + GATE_COMPAT_VERSION + `"`}
		CFLAGS   = []string{"-O2", "-fomit-frame-pointer", "-g", "-Wall", "-Wextra", "-Wno-unused-parameter", "-Wimplicit-fallthrough"}
		CXXFLAGS = []string{"-std=c++17", "-fno-exceptions", "-Wno-write-strings"}
		LDFLAGS  = []string{"-static"}

		source = "runtime/executor/executor.cpp"
		binary = "lib/gate/gate-runtime-executor." + GATE_COMPAT_MAJOR
	)

	return If(
		Outdated(binary, Glob(
			source,
			"runtime/include/*.hpp",
		)...),
		Func(func() error {
			return os.MkdirAll(path.Dir(binary), 0777)
		}),
		Command(CXX, CPPFLAGS, CFLAGS, CXXFLAGS, LDFLAGS, "-o", binary, source),
	)
}

func loaderTask(CXX string) Task {
	var (
		CPPFLAGS = []string{"-Iruntime/loader/" + arch, "-Iruntime/include/" + arch, "-Iruntime/include", "-DNDEBUG", "-DPIE", "-DGATE_LOADER_ADDR=0x200000000"}
		CFLAGS   = []string{"-O2", "-fPIE", "-fomit-frame-pointer", "-fno-stack-protector", "-g", "-Wall", "-Wextra", "-Wno-unused-parameter", "-Wimplicit-fallthrough"}
		CXXFLAGS = []string{"-std=c++17", "-fno-exceptions"}
		LDFLAGS  = []string{"-static", "-nostartfiles", "-nostdlib", "-Wl,-z,noexecstack", "-Wl,-Ttext-segment=0x200000000", "-Wl,--build-id=none"}

		start    = "runtime/loader/" + arch + "/start."
		runtime2 = "runtime/loader/" + arch + "/runtime2."
		loader   = "runtime/loader/loader."
		binary   = "lib/gate/gate-runtime-loader." + GATE_COMPAT_VERSION
	)

	return Join(
		If(
			Outdated(start+o, start+S),
			Command(CXX, CPPFLAGS, "-c", "-o", start+o, start+S),
		),
		If(
			Outdated(runtime2+o, Glob(
				runtime2+S,
				"runtime/loader/"+arch+"/runtime.S",
				"runtime/loader/poll.S",
				"runtime/loader/seccomp.S",
				"runtime/include/"+arch+"/*.hpp",
				"runtime/include/*.hpp",
			)...),
			Command(CXX, CPPFLAGS, "-c", "-o", runtime2+o, runtime2+S),
		),
		If(
			Outdated(loader+o, Glob(
				loader+cpp,
				"runtime/include/loader/"+arch+"/*.hpp",
				"runtime/include/"+arch+"/*.hpp",
				"runtime/include/*.hpp",
			)...),
			Command(CXX, CPPFLAGS, CFLAGS, CXXFLAGS, LDFLAGS, "-c", "-o", loader+o, loader+cpp),
		),
		Func(func() error {
			return os.MkdirAll(path.Dir(binary), 0777)
		}),
		If(
			Outdated(binary, start+o, loader+o, runtime2+o),
			Command(CXX, CFLAGS, CXXFLAGS, LDFLAGS, "-o", binary, start+o, loader+o, runtime2+o),
		),
	)
}

func benchmarkTask(GO, TAGS string) Task {
	var (
		PERFLOCK  = Option("PERFLOCK", "perflock")
		BENCHSTAT = Option("BENCHSTAT", "benchstat")
	)

	BENCH := Option("BENCH", ".")
	BENCHFLAGS := []string{
		"-bench=" + BENCH,
		"-tags=" + TAGS,
	}

	return Func(func() error {
		args := Flatten(GO, "test", "-run=-", BENCHFLAGS, "./...")
		if PERFLOCK != "" {
			args = append([]string{PERFLOCK}, args...)
		}

		Println(args...)

		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stderr = os.Stderr
		output, err := cmd.Output()
		os.Stdout.Write(output)
		if err != nil {
			return err
		}

		if err := ioutil.WriteFile("bench-new.txt", output, 0666); err != nil {
			return err
		}

		if _, err := os.Stat("bench-old.txt"); err == nil {
			cmd := exec.Command(BENCHSTAT, "bench-old.txt", "bench-old.txt")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				return err
			}
		}

		return nil
	})
}
