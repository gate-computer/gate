// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package router

import (
	"context"
	"net/http"
)

// Router for registering HTTP request handlers.  Works like net/http.ServeMux.
type Router interface {
	Handle(pattern string, handler http.Handler)
	HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request))
}

type key struct{}

// Context with the given router.
func Context(ctx context.Context, r Router) context.Context {
	return context.WithValue(ctx, key{}, r)
}

// Contextual router or nil.
func Contextual(ctx context.Context) Router {
	r, _ := ctx.Value(key{}).(Router)
	return r
}

// MustHandle panics if context has no router.
func MustHandle(ctx context.Context, pattern string, handler http.Handler) {
	r := ctx.Value(key{}).(Router)
	r.Handle(pattern, handler)
}

// MustHandleFunc panics if context has no router.
func MustHandleFunc(ctx context.Context, pattern string, handler func(http.ResponseWriter, *http.Request)) {
	r := ctx.Value(key{}).(Router)
	r.HandleFunc(pattern, handler)
}
