// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"

	"github.com/tsavola/gate/server/detail"
)

type contextKey struct{}

func WithIface(ctx context.Context, iface detail.Iface) context.Context {
	c := Context(ctx)
	c.Iface = iface
	return context.WithValue(ctx, contextKey{}, c)
}

func WithClient(ctx context.Context, client string) context.Context {
	c := Context(ctx)
	c.Client = client
	return context.WithValue(ctx, contextKey{}, c)
}

func WithCall(ctx context.Context, call string) context.Context {
	c := Context(ctx)
	c.Call = call
	return context.WithValue(ctx, contextKey{}, c)
}

func Context(ctx context.Context) (c detail.Context) {
	if x := ctx.Value(contextKey{}); x != nil {
		c = x.(detail.Context)
	}
	return
}
