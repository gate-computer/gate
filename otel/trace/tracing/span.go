// Copyright (c) 2025 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tracing

import (
	"net/http"

	"gate.computer/gate/server/api"
	"gate.computer/otel/trace/tracelink"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	. "import.name/type/context"
)

// Tracer is a less restrictive edition of [trace.Tracer].
type Tracer interface {
	Start(ctx Context, spanName string, opts ...trace.SpanStartOption) (Context, trace.Span)
}

// HTTPSpanStarter returns a function which can be used as
// webserver.Config.StartSpan callback.
func HTTPSpanStarter(tracer Tracer) func(*http.Request, string) (Context, func(Context)) {
	if tracer == nil {
		tracer = otel.GetTracerProvider().Tracer("gate")
	}
	return func(r *http.Request, pattern string) (Context, func(Context)) {
		return startSpan(r.Context(), tracer, pattern, trace.SpanKindServer)
	}
}

// SpanStarter returns a function which can be used as server.Config.StartSpan
// callback.
func SpanStarter(tracer Tracer) func(Context, api.Op) (Context, func(Context)) {
	if tracer == nil {
		tracer = otel.GetTracerProvider().Tracer("gate")
	}
	return func(ctx Context, op api.Op) (Context, func(Context)) {
		return startSpan(ctx, tracer, op.String(), trace.SpanKindInternal)
	}
}

func startSpan(ctx Context, tracer Tracer, name string, kind trace.SpanKind) (Context, func(Context)) {
	ctx, links := tracelink.RemoveLinksFromContext(ctx)
	ctx, span := tracer.Start(ctx, name, trace.WithSpanKind(kind), trace.WithLinks(links...))

	// TODO: attributes

	return ctx, func(ctx Context) {
		span.End()
	}
}

func TraceDetacher() func(Context) Context {
	return detachTrace
}

func detachTrace(ctx Context) Context {
	link := trace.LinkFromContext(ctx)
	ctx = trace.ContextWithSpanContext(ctx, trace.SpanContext{})
	return tracelink.ContextWithLinks(ctx, link)
}
