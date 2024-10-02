// Copyright (c) 2020 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package principal

import (
	"context"

	. "import.name/type/context"
)

type contextInstanceUUIDValueKey struct{}

func ContextWithInstanceUUID(ctx Context, id [16]byte) Context {
	return context.WithValue(ctx, contextInstanceUUIDValueKey{}, id)
}

func ContextInstanceUUID(ctx Context) (id [16]byte, ok bool) {
	id, ok = ctx.Value(contextInstanceUUIDValueKey{}).([16]byte)
	return
}

func MustContextInstanceUUID(ctx Context) [16]byte {
	return ctx.Value(contextInstanceUUIDValueKey{}).([16]byte)
}
