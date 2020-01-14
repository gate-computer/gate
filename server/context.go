// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"

	"github.com/tsavola/gate/internal/principal"
	"github.com/tsavola/gate/server/detail"
)

type contextKey struct{}

func ContextWithIface(ctx context.Context, iface detail.Iface) context.Context {
	c := Context(ctx, nil)
	c.Iface = iface
	return context.WithValue(ctx, contextKey{}, c)
}

func ContextWithRequestAddr(ctx context.Context, request uint64, addr string) context.Context {
	c := Context(ctx, nil)
	c.Req = request
	c.Addr = addr
	return context.WithValue(ctx, contextKey{}, c)
}

func ContextWithOp(ctx context.Context, op detail.Op) context.Context {
	c := Context(ctx, nil)
	c.Op = op
	return context.WithValue(ctx, contextKey{}, c)
}

func detachedContext(ctx context.Context, pri *principal.ID) context.Context {
	c := Context(ctx, pri)
	c.Addr = ""
	return context.WithValue(context.Background(), contextKey{}, c)
}

func Context(ctx context.Context, pri *principal.ID) (c detail.Context) {
	if x := ctx.Value(contextKey{}); x != nil {
		c = x.(detail.Context)
	}

	if pri != nil {
		c.Principal = pri.String()
	}

	return
}

// ContextOp returns the server operation type.
func ContextOp(ctx context.Context) (op detail.Op) {
	if x := ctx.Value(contextKey{}); x != nil {
		op = x.(detail.Context).Op
	}
	return
}
