// Copyright (c) 2024 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package trace

import (
	"context"
	"encoding/hex"
	"slices"

	. "import.name/type/context"
)

type (
	TraceID [16]byte
	SpanID  [8]byte
)

func (id TraceID) String() string { return hex.EncodeToString(id[:]) }
func (id SpanID) String() string  { return hex.EncodeToString(id[:]) }

type contextKey int

var (
	contextTraceID   any = contextKey(0)
	contextSpanID    any = contextKey(1)
	contextAutoLinks any = contextKey(2)
)

func ContextWithoutTrace(ctx Context) Context {
	ctx = context.WithValue(ctx, contextTraceID, nil)
	ctx = context.WithValue(ctx, contextSpanID, nil)
	return ctx
}

func ContextWithTraceID(ctx Context, id TraceID) Context {
	return context.WithValue(ctx, contextTraceID, id)
}

func ContextWithSpanID(ctx Context, id SpanID) Context {
	return context.WithValue(ctx, contextSpanID, id)
}

func ContextTraceID(ctx Context) (TraceID, bool) {
	id, ok := ctx.Value(contextTraceID).(TraceID)
	return id, ok
}

func ContextSpanID(ctx Context) (SpanID, bool) {
	id, ok := ctx.Value(contextSpanID).(SpanID)
	return id, ok
}

type Link struct {
	TraceID TraceID
	SpanID  SpanID
}

func LinkToContext(ctx Context) *Link {
	if traceID, ok := ContextTraceID(ctx); ok {
		if spanID, ok := ContextSpanID(ctx); ok {
			return &Link{traceID, spanID}
		}
	}
	return nil
}

func ContextWithAutoLinks(ctx Context, links ...*Link) Context {
	links = slices.DeleteFunc(links, func(l *Link) bool { return l == nil })
	return context.WithValue(ctx, contextAutoLinks, links)
}

func ContextAutoLinks(ctx Context) []*Link {
	links, _ := ctx.Value(contextAutoLinks).([]*Link)
	return links
}
