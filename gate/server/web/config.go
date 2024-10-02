// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package web

import (
	"net/http"
	"time"

	"gate.computer/gate/server/api"
	"gate.computer/gate/server/event"

	. "import.name/type/context"
)

type NonceChecker interface {
	CheckNonce(ctx Context, scope []byte, nonce string, expires time.Time) error
}

// Config for a web server.
type Config struct {
	Server       api.Server
	Authority    string   // External domain name with optional port number.
	Origins      []string // Value "*" causes Origin header to be ignored.
	NonceStorage NonceChecker
	NewRequestID func(*http.Request) uint64
	Monitor      func(*event.Event, error)
}

func (c *Config) Configured() bool {
	return c.Server != nil && c.Authority != "" && len(c.Origins) != 0
}

func (c *Config) monitorError(ctx Context, t event.Type, err error) {
	c.Monitor(&event.Event{
		Type: t,
		Meta: api.ContextMeta(ctx),
	}, err)
}

func (c *Config) monitorFail(ctx Context, t event.Type, info *event.Fail, err error) {
	c.Monitor(&event.Event{
		Type: t,
		Meta: api.ContextMeta(ctx),
		Info: &event.EventFail{Fail: info},
	}, err)
}
