// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package system

import (
	"context"
)

const Scope = "program:system"

type contextKey int

const (
	contextUID contextKey = iota
)

func ContextWithUserID(ctx context.Context, uid string) context.Context {
	return context.WithValue(ctx, contextUID, uid)
}

func ContextUserID(ctx context.Context) string {
	s, _ := ctx.Value(contextUID).(string)
	return s
}
