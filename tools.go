// Copyright (c) 2024 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build tools

package gate

import (
	_ "gate.computer/ga"
	_ "github.com/bufbuild/buf/cmd/buf"
	_ "google.golang.org/grpc/cmd/protoc-gen-go-grpc"
)
