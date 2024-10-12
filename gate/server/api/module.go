// Copyright (c) 2022 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"crypto"
	"encoding/hex"
)

const (
	KnownModuleSource = "sha256"
	KnownModuleHash   = crypto.SHA256
)

func EncodeKnownModule(hashSum []byte) string {
	return hex.EncodeToString(hashSum)
}
