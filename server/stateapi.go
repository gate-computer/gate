// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"

	internal "github.com/tsavola/gate/internal/server"
)

const (
	DefaultMemorySizeLimit = internal.DefaultMemorySizeLimit
	DefaultStackSize       = internal.DefaultStackSize
	DefaultPreforkProcs    = internal.DefaultPreforkProcs
)

type Origin = internal.Origin
type Server = internal.Server
type Config = internal.Config

type State struct {
	Internal internal.State
}

// NewState retains significant resources until the context is canceled.
func NewState(ctx context.Context, config *Config) *State {
	s := new(State)
	s.Internal.Init(ctx, *config)
	return s
}
