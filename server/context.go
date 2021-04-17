// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"

	"gate.computer/gate/internal/principal"
	"gate.computer/gate/server/detail"
	"google.golang.org/protobuf/proto"
)

type contextKey struct{}

func ContextWithIface(ctx context.Context, iface detail.Iface) context.Context {
	c := ContextDetail(ctx)
	c.Iface = iface
	return context.WithValue(ctx, contextKey{}, c)
}

func ContextWithRequestAddr(ctx context.Context, request uint64, addr string) context.Context {
	c := ContextDetail(ctx)
	c.Req = request
	c.Addr = addr
	return context.WithValue(ctx, contextKey{}, c)
}

func ContextWithOp(ctx context.Context, op detail.Op) context.Context {
	c := ContextDetail(ctx)
	c.Op = op
	return context.WithValue(ctx, contextKey{}, c)
}

func detachedContext(ctx context.Context) context.Context {
	c := ContextDetail(ctx)
	c.Addr = ""
	return context.WithValue(ctx, contextKey{}, c)
}

func ContextDetail(ctx context.Context) (c *detail.Context) {
	pri := principal.ContextID(ctx)

	var priString string
	if pri != nil {
		priString = pri.String()
	}

	if x := ctx.Value(contextKey{}); x != nil {
		c = x.(*detail.Context)
		if pri != nil && c.Principal != priString {
			c = proto.Clone(c).(*detail.Context)
			c.Principal = priString
		}
	} else {
		c = new(detail.Context)
		if pri != nil {
			c.Principal = priString
		}
	}
	return
}

// ContextOp returns the server operation type.
func ContextOp(ctx context.Context) (op detail.Op) {
	if x := ctx.Value(contextKey{}); x != nil {
		op = x.(*detail.Context).Op
	}
	return
}
