// Copyright (c) 2024 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"fmt"
	"maps"
	"net/http"

	"go.opentelemetry.io/otel/trace"
)

func httpDoPropagateTraceContext(r *http.Request) (*http.Response, error) {
	if c := trace.SpanContextFromContext(r.Context()); c.IsValid() {
		h := maps.Clone(r.Header)
		h.Set("Traceparent", fmt.Sprintf("00-%s-%s-%s", c.TraceID(), c.SpanID(), c.TraceFlags()))
		r.Header = h
	}

	return http.DefaultClient.Do(r)
}
