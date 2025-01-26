// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webserver

import (
	"crypto/ed25519"
	"errors"
	"net/http"

	"gate.computer/gate/server/api"
	"gate.computer/gate/server/event"
	"gate.computer/gate/server/model"

	. "import.name/type/context"
)

// Config for a web server.
type Config struct {
	Server       api.Server
	Authority    string   // External domain name with optional port number.
	Origins      []string // Value "*" causes Origin header to be ignored.
	NonceChecker model.NonceChecker

	// StartSpan within request context, ending when endSpan is called.  See
	// gate.computer/gate/trace/tracelink.  The pattern string indicates the
	// matching HTTP route handler.
	//
	// TODO: is pattern redundant?
	StartSpan func(r *http.Request, pattern string) (ctx Context, endSpan func(Context))

	// AddEvent to the current trace span, or outside of trace but in relation
	// to span links.  See gate.computer/gate/trace/tracelink.
	AddEvent func(Context, *event.Event, error)

	// DetachTrace is invoked after a potentially long-running connection has
	// finished its setup.  It should end current trace and/or span if
	// possible, and prepare the context for linking to the ended span.  See
	// gate.computer/gate/trace/tracelink.
	DetachTrace func(Context) Context

	identityKey *ed25519.PrivateKey
}

func (c *Config) SetIdentityKey(privateKey any) error {
	key, ok := privateKey.(*ed25519.PrivateKey)
	if !ok {
		return errors.New("server identity key type not supported")
	}
	c.identityKey = key
	return nil
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
