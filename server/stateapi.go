package server

import (
	"context"

	internal "github.com/tsavola/gate/internal/server"
	"github.com/tsavola/gate/server/serverconfig"
)

type State struct {
	Internal internal.State
}

func NewState(ctx context.Context, opt serverconfig.Options, set serverconfig.Settings) *State {
	s := new(State)
	s.Internal.Init(ctx, opt, set)
	return s
}
