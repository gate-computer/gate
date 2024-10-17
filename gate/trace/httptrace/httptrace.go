// Copyright (c) 2024 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package httptrace

import (
	"fmt"
	"maps"
	"net/http"

	"gate.computer/gate/trace"

	. "import.name/type/context"
)

var DefaultClient = http.DefaultClient

// DoPropagate calls DefaultClient.Do.  If trace context is found, the request
// header is modified by adding "traceparent".
func DoPropagate(r *http.Request) (*http.Response, error) {
	if s := ContextTraceParent(r.Context()); s != "" {
		h := maps.Clone(r.Header)
		h["traceparent"] = []string{s}
		r.Header = h
	}
	return DefaultClient.Do(r)
}

// ContextTraceParent returns a string for use as "traceparent" header, or
// empty string.
func ContextTraceParent(ctx Context) string {
	if traceID, ok := trace.ContextTraceID(ctx); ok {
		if spanID, ok := trace.ContextSpanID(ctx); ok {
			return fmt.Sprintf("00-%s-%s-00", traceID, spanID)
		}
	}
	return ""
}
