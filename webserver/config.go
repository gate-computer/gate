// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webserver

const (
	DefaultMaxProgramSize = 16777216
)

type Config struct {
	MaxProgramSize int
}
