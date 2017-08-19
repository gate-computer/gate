// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"

	internal "github.com/tsavola/gate/internal/server"
	"github.com/tsavola/gate/server/serverconfig"
)

type State struct {
	Internal internal.State
}

// NewState retains significant resources until the context is canceled.
func NewState(ctx context.Context, config *serverconfig.Config) *State {
	s := new(State)
	s.Internal.Init(ctx, *config)
	return s
}
