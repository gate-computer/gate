// Copyright (c) 2024 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tracelink

import (
	"context"
	"slices"

	"go.opentelemetry.io/otel/trace"

	. "import.name/type/context"
)

type contextKey struct{}

var contextLinks any = contextKey{}

func ContextWithLinks(ctx Context, links ...trace.Link) Context {
	links = slices.DeleteFunc(links, func(l trace.Link) bool {
		return !l.SpanContext.IsValid()
	})

	return context.WithValue(ctx, contextLinks, slices.Clip(links))
}

// RemoveLinksFromContext returns the links.
func RemoveLinksFromContext(ctx Context) (Context, []trace.Link) {
	links, _ := ctx.Value(contextLinks).([]trace.Link)
	if len(links) == 0 {
		return ctx, nil
	}

	ctx = context.WithValue(ctx, contextLinks, nil)
	return ctx, links
}
