// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package system

import (
	"context"

	"gate.computer/gate/scope"

	. "import.name/type/context"
)

const Scope = "program:system"

func init() {
	scope.Register(Scope)
}

type contextKey int

const (
	contextUID contextKey = iota
)

func ContextWithUserID(ctx Context, uid string) Context {
	return context.WithValue(ctx, contextUID, uid)
}

func ContextUserID(ctx Context) string {
	s, _ := ctx.Value(contextUID).(string)
	return s
}
