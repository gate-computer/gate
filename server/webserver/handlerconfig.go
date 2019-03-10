// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webserver

import (
	"context"
	"net/http"
	"time"

	"github.com/tsavola/gate/server"
)

type NonceChecker interface {
	CheckNonce(ctx context.Context, pri *server.PrincipalKey, nonce string, expires time.Time) error
}

// Config for a web server.
type Config struct {
	Server        *server.Server
	Authority     string // External domain name with optional port number.
	NonceStorage  NonceChecker
	ModuleSources map[string]server.Source
	NewRequestID  func(*http.Request) uint64
}

func (c *Config) Configured() bool {
	return c.Authority != ""
}
