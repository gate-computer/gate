// Copyright (c) 2025 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package catalog

import (
	"log/slog"
	"sync"

	"gate.computer/uapi/service"
)

var srv = sync.OnceValue(func() *service.Service {
	return service.MustRegister("catalog", func([]byte) {
		slog.Debug("gate: catalog: info packet received")
	})
})

// JSON document describing available services.
func JSON() <-chan []byte {
	c := make(chan []byte, 1)
	srv().Call([]byte("json"), func(reply []byte) {
		c <- reply
	})
	return c
}
