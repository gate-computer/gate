// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"

	"gate.computer/gate/internal/principal"
	"gate.computer/gate/server/detail"
)

type contextKey int

const (
	contextKeyDetail contextKey = iota
	contextKeyScope
)

func ContextWithIface(ctx context.Context, iface detail.Iface) context.Context {
	c := ContextDetail(ctx)
	c.Iface = iface
	return context.WithValue(ctx, contextKeyDetail, c)
}

func ContextWithRequestAddr(ctx context.Context, request uint64, addr string) context.Context {
	c := ContextDetail(ctx)
	c.Req = request
	c.Addr = addr
	return context.WithValue(ctx, contextKeyDetail, c)
}

func ContextWithOp(ctx context.Context, op detail.Op) context.Context {
	c := ContextDetail(ctx)
	c.Op = op
	return context.WithValue(ctx, contextKeyDetail, c)
}

func ContextWithScope(ctx context.Context, scope []string) context.Context {
	if len(scope) == 0 {
		return ctx
	}
	return context.WithValue(ctx, contextKeyScope, scope)
}

func detachedContext(ctx context.Context) context.Context {
	c := ContextDetail(ctx)
	c.Addr = ""
	return context.WithValue(ctx, contextKeyDetail, c)
}

func ContextDetail(ctx context.Context) (c detail.Context) {
	if x := ctx.Value(contextKeyDetail); x != nil {
		c = x.(detail.Context)
	}

	if pri := principal.ContextID(ctx); pri != nil {
		c.Principal = pri.String()
	}

	return
}

// ContextOp returns the server operation type.
func ContextOp(ctx context.Context) (op detail.Op) {
	if x := ctx.Value(contextKeyDetail); x != nil {
		op = x.(detail.Context).Op
	}
	return
}

func ScopeContains(ctx context.Context, scope string) bool {
	x := ctx.Value(contextKeyScope)
	if x == nil {
		return false
	}

	for _, s := range x.([]string) {
		if s == scope {
			return true
		}
	}

	return false
}
