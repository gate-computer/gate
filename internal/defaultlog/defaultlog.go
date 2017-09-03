// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package defaultlog

import (
	"log"
)

type StandardLogger struct{}

func (StandardLogger) Printf(fmt string, v ...interface{}) {
	log.Printf(fmt, v...)
}

type NoLogger struct{}

func (NoLogger) Printf(string, ...interface{}) {
}
