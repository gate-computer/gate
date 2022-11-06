// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package include embeds C header files.
package include

import (
	"embed"
)

//go:embed *.h
var FS embed.FS
