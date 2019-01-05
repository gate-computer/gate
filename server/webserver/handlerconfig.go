// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webserver

import (
	"net/http"

	"github.com/tsavola/gate/server"
	"github.com/tsavola/gate/server/state"
)

// Config for a web server.
type Config struct {
	Server        *server.Server
	Authority     string              // External domain name with optional port number.
	AccessState   state.AccessTracker // Remembers things within the Authority.
	ModuleSources map[string]server.Source
	NewRequestID  func(*http.Request) uint64
}

func (c *Config) Configured() bool {
	return c.Authority != "" && c.AccessState != nil
}
