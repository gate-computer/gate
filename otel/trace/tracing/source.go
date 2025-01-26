// Copyright (c) 2025 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tracing

import (
	"io"

	"gate.computer/gate/source"
	"gate.computer/otel/trace/tracelink"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	. "import.name/type/context"
)

func Source(s source.Source, tracer Tracer) source.Source {
	if tracer == nil {
		tracer = otel.GetTracerProvider().Tracer("gate")
	}
	return &tracingSource{s, tracer}
}

type tracingSource struct {
	source.Source
	tracer Tracer
}

func (s *tracingSource) OpenURI(ctx Context, uri string, maxSize int) (io.ReadCloser, int64, error) {
	ctx, links := tracelink.RemoveLinksFromContext(ctx)
	ctx, span := s.tracer.Start(ctx, uri, trace.WithSpanKind(trace.SpanKindClient), trace.WithLinks(links...))
	defer span.End()

	// TODO: attributes

	r, n, err := s.Source.OpenURI(ctx, uri, maxSize)

	switch {
	case err != nil:
		span.SetStatus(codes.Error, err.Error())
	case r == nil && n != 0:
		span.SetStatus(codes.Error, "content too long")
	case r == nil && n == 0:
		span.SetStatus(codes.Error, "not found")
	default:
		span.SetStatus(codes.Ok, "")
	}

	return r, n, err
}
