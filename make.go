// Copyright (c) 2022 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build generate
// +build generate

package main

//go:generate go run make.go generate

import (
	. "import.name/make"
)

func main() { Main(targets, "make.go", "go.mod") }

func targets() (targets Tasks) {
	GO := Getvar("GO", "go")

	sources := generate(GO)
	targets.Add(Target("generate", sources))
	targets.Add(Target("check", sources, check(GO)))
	targets.Add(Target("clean", Removal("lib")))
	return
}

func generate(GO string) Task {
	protos := Globber(
		"api/*.proto",
	)

	tasks := Tasks{
		Command(GO, "build", "-o", "lib/", "google.golang.org/grpc/cmd/protoc-gen-go-grpc"),
	}

	for _, proto := range protos() {
		tasks.Add(If(Outdated(ReplaceSuffix(proto, ".pb.go"), protos),
			Command(GO, "run", "github.com/bufbuild/buf/cmd/buf", "generate", "--path", proto),
		))
	}

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
