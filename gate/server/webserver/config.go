// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webserver

import (
	"net/http"

	"gate.computer/gate/server/api"
	"gate.computer/gate/server/event"
	"gate.computer/gate/server/model"
	"gate.computer/gate/trace"

	. "import.name/type/context"
)

// Config for a web server.
type Config struct {
	Server       api.Server
	Authority    string   // External domain name with optional port number.
	Origins      []string // Value "*" causes Origin header to be ignored.
	NonceChecker model.NonceChecker

	// StartSpan within request context, ending when endSpan is called.  Nil
	// links must be ignored.  [trace.ContextAutoLinks] must also be respected.
	// The pattern string indicates the matching HTTP route handler.
	StartSpan func(r *http.Request, pattern string, links ...*trace.Link) (ctx Context, endSpan func(Context))

	// AddEvent to the current trace span, or outside of trace but in relation
	// to [trace.ContextAutoLinks].
	AddEvent func(Context, *event.Event, error)
}

func (c *Config) Configured() bool {
	return c.Server != nil && c.Authority != "" && len(c.Origins) != 0
}

func (c *Config) event(ctx Context, t event.Type, err error) {
	c.AddEvent(ctx, &event.Event{
		Type: t,
	}, err)
}

func (c *Config) eventFail(ctx Context, t event.Type, info *event.Fail, err error) {
	c.AddEvent(ctx, &event.Event{
		Type: t,
		Info: &event.EventFail{Fail: info},
	}, err)
}
