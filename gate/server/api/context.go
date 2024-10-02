// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"

	"gate.computer/gate/server/api/pb"
	"gate.computer/internal/principal"

	. "import.name/type/context"
)

type contextKey int

const (
	contextKeyIface = contextKey(iota)
	contextKeyReq
	contextKeyAddr
	contextKeyOp
)

type (
	Iface = pb.Iface
	Op    = pb.Op
	Meta  = pb.Meta
)

func ContextWithMeta(ctx Context, m *Meta) (Context, error) {
	if m.Iface != 0 {
		ctx = ContextWithIface(ctx, m.Iface)
	}

	if m.Req != 0 {
		ctx = ContextWithRequest(ctx, m.Req)
	}

	if m.Addr != "" {
		ctx = ContextWithAddress(ctx, m.Addr)
	}

	if m.Op != 0 {
		ctx = ContextWithOp(ctx, m.Op)
	}

	if m.Principal != "" {
		id, err := principal.ParseID(m.Principal)
		if err != nil {
			return ctx, err
		}
		ctx = principal.ContextWithID(ctx, id)
	}

	return ctx, nil
}

func ContextWithIface(ctx Context, iface Iface) Context {
	return context.WithValue(ctx, contextKeyIface, iface)
}

func ContextWithRequest(ctx Context, req uint64) Context {
	return context.WithValue(ctx, contextKeyReq, req)
}

func ContextWithAddress(ctx Context, addr string) Context {
	return context.WithValue(ctx, contextKeyAddr, addr)
}

func ContextWithOp(ctx Context, op Op) Context {
	return context.WithValue(ctx, contextKeyOp, op)
}

// ContextOp returns the server operation type.
func ContextOp(ctx Context) (op Op) {
	if x := ctx.Value(contextKeyOp); x != nil {
		op = x.(Op)
	}
	return
}

func ContextMeta(ctx Context) *Meta {
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
