// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"

	"gate.computer/gate/internal/principal"
)

type contextKey int

const (
	contextKeyIface = contextKey(iota)
	contextKeyReq
	contextKeyAddr
	contextKeyOp
)

func ContextWithIface(ctx context.Context, iface Iface) context.Context {
	return context.WithValue(ctx, contextKeyIface, iface)
}

func ContextWithRequest(ctx context.Context, req uint64) context.Context {
	return context.WithValue(ctx, contextKeyReq, req)
}

func ContextWithAddress(ctx context.Context, addr string) context.Context {
	return context.WithValue(ctx, contextKeyAddr, addr)
}

func ContextWithOp(ctx context.Context, op Op) context.Context {
	return context.WithValue(ctx, contextKeyOp, op)
}

// ContextOp returns the server operation type.
func ContextOp(ctx context.Context) (op Op) {
	if x := ctx.Value(contextKeyOp); x != nil {
		op = x.(Op)
	}
	return
}

func ContextMeta(ctx context.Context) *Meta {
	m := new(Meta)

	if x := ctx.Value(contextKeyIface); x != nil {
		m.Iface = x.(Iface)
	}

	if x := ctx.Value(contextKeyReq); x != nil {
		m.Req = x.(uint64)
	}

	if x := ctx.Value(contextKeyAddr); x != nil {
		m.Addr = x.(string)
	}

	m.Op = ContextOp(ctx)

	if pri := principal.ContextID(ctx); pri != nil {
		m.Principal = pri.String()
	}

	return m
}
