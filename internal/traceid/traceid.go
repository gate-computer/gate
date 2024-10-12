// Copyright (c) 2024 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package traceid

import (
	cryptorand "crypto/rand"
	mathrand "math/rand/v2"
	"sync"

	"gate.computer/gate/trace"
)

var (
	mu     sync.Mutex
	random *mathrand.ChaCha8
)

func init() {
	var seed [32]byte
	if _, err := cryptorand.Read(seed[:]); err != nil {
		panic(err)
	}
	random = mathrand.NewChaCha8(seed)
}

func MakeTraceID() (id trace.TraceID) {
	mu.Lock()
	defer mu.Unlock()
	random.Read(id[:])
	return
}

func MakeSpanID() (id trace.SpanID) {
	mu.Lock()
	defer mu.Unlock()
	random.Read(id[:])
	return
}
