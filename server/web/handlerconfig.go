// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package web

import (
	"context"
	"net/http"
	"time"

	"gate.computer/gate/server"
)

type NonceChecker interface {
	CheckNonce(ctx context.Context, scope []byte, nonce string, expires time.Time) error
}

// Config for a web server.
type Config struct {
	Server        *server.Server
	Authority     string   // External domain name with optional port number.
	Origins       []string // Value "*" causes Origin header to be ignored.
	NonceStorage  NonceChecker
	ModuleSources map[string]server.Source
	NewRequestID  func(*http.Request) uint64
}

func (c Config) Configured() bool {
	return c.Server != nil && c.Authority != "" && len(c.Origins) != 0
}
