// Copyright (c) 2024 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webserver

import (
	"context"

	. "import.name/type/context"
)

type contextEndSpanKey struct{}

var contextEndSpan any = contextEndSpanKey{}

type spanEnder struct {
	ctx Context
	f   func()
}

// contextWithSpanEnding returns a function which must be called instead of
// endSpan.
func contextWithSpanEnding(ctx Context, endSpan func(Context)) (Context, func()) {
	ender := new(spanEnder)
	ctx = context.WithValue(ctx, contextEndSpan, ender)

	f := func() {
		if ctx := ender.ctx; ctx != nil {
			ender.ctx = nil
			endSpan(ctx)
		}
	}

	ender.ctx = ctx
	ender.f = f

	return ctx, f
}

func endContextSpan(ctx Context) {
	if x := ctx.Value(contextEndSpan); x != nil {
		x.(*spanEnder).f()
	}
}
