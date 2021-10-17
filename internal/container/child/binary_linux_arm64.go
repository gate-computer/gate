// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !gateexecdir
// +build !gateexecdir

package child

import (
	_ "embed"
)

//go:embed binary/executor.linux-arm64.gz
var executorEmbed []byte

//go:embed binary/loader.linux-arm64.gz
var loaderEmbed []byte
