// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtimeapi

type Logger interface {
	Printf(string, ...interface{})
}
