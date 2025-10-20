// Copyright (c) 2025 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package scope

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"gate.computer/uapi/service"
)

// Represents system access.
const System = "program:system"

const (
	callRestrict uint8 = 0
)

var srv = sync.OnceValue(func() *service.Service {
	return service.MustRegister("scope", func([]byte) {
		slog.Debug("gate: scope: info packet received")
	})
})

// MustRestrict synchronously and panic on error.
func MustRestrict(scope []string) {
	if err := <-Restrict(scope); err != nil {
		panic(err)
	}
}

// Restrict execution privileges to the specified set.  Privileges cannot be
// added; each invocation can only remove privileges (extraneous scope is
// ignored).  Actual privileges depend also on the execution environment, and
// may vary during program execution.
func Restrict(scope []string) <-chan error {
	c := make(chan error, 1)

	if len(scope) > 255 {
		c <- errors.New("scope is too large")
		return c
	}

	size := 1 + 1
	for _, s := range scope {
		if len(s) > 255 {
			c <- errors.New("scope string is too long")
			return c
		}
		size += 1 + len(s)
	}

	b := bytes.NewBuffer(make([]byte, 0, size))
	b.WriteByte(callRestrict)
	b.WriteByte(uint8(len(scope)))
	for _, s := range scope {
		b.WriteByte(uint8(len(s)))
		b.WriteString(s)
	}

	srv().Call(b.Bytes(), func(reply []byte) {
		if len(reply) == 0 {
			c <- errors.New("unknown scope service call")
			return
		}

		if error := binary.LittleEndian.Uint16(reply); error != 0 {
			c <- fmt.Errorf("unknown scope service call error %d", error)
			return
		}

		c <- nil
	})

	return c
}
