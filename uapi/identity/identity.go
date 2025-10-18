// Copyright (c) 2025 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package identity

import (
	"log/slog"
	"sync"

	"gate.computer/uapi/service"
)

const (
	callPrincipalID uint8 = 0
	callInstanceID  uint8 = 1
)

var srv = sync.OnceValue(func() *service.Service {
	return service.MustRegister("identity", func([]byte) {
		slog.Debug("gate: identity: info packet received")
	})
})

func PrincipalID() <-chan string {
	return getID(callPrincipalID)
}

func InstanceID() <-chan string {
	return getID(callInstanceID)
}

func getID(call uint8) <-chan string {
	c := make(chan string)
	srv().Call([]byte{call}, func(reply []byte) {
		c <- string(reply)
	})
	return c
}
