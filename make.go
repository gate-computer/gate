// Copyright (c) 2022 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build generate
// +build generate

package main

//go:generate go run make.go generate

import (
	"strings"

	. "import.name/make"
)

func main() { Main(targets, "make.go", "go.mod") }

func targets() (targets Tasks) {
	var (
		GO     = Getvar("GO", "go")
		PROTOC = Getvar("PROTOC", "protoc")
	)

	sources := generate(GO, PROTOC)
	targets.Add(Target("generate", sources))
	targets.Add(Target("check", sources, check(GO)))
	targets.Add(Target("clean", Removal("lib")))
	return
}

func generate(GO, PROTOC string) Task {
	var (
		deps = Globber(
			"api/*.proto",
		)
	)

	var tasks Tasks

	addPlugin := func(pkg string) {
		plugin := Join("lib", Base(pkg))

		tasks.Add(If(Outdated(plugin, nil),
			Command(GO, "build", "-o", plugin, pkg)),
		)
	}

	addPlugin("google.golang.org/protobuf/cmd/protoc-gen-go")
	addPlugin("google.golang.org/grpc/cmd/protoc-gen-go-grpc")

	addProto := func(proto, supplement string) {
		var (
			plugin = Join("lib/protoc-gen-go" + supplement)
			suffix = strings.Replace(supplement, "-", "_", 1)
			gen    = ReplaceSuffix(proto, suffix+".pb.go")
		)

		tasks.Add(If(Outdated(gen, Flattener(deps, plugin)),
			Command(PROTOC,
				"--plugin="+plugin,
				"--go"+supplement+"_out=.",
				"--go"+supplement+"_opt=paths=source_relative",
				proto,
			),
		))
	}

	addProto("api/service.proto", "")
	addProto("api/service.proto", "-grpc")

	return Group(tasks...)
}

func check(GO string) Task {
	return Group(
		Command(GO, "build", "-o", "/dev/null", "./..."),
		Command(GO, "vet", "./..."),
		Command(GO, "build", "-o", "lib/", "./internal/test-service"),
		Command(GO, "test", "-v", "./..."),
	)
}
