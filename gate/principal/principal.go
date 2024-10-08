// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package principal

import (
	internal "gate.computer/internal/principal"

	. "import.name/type/context"
)

type (
	ID   = internal.ID
	Type = internal.Type
)

const (
	TypeLocal   Type = internal.TypeLocal
	TypeEd25519      = internal.TypeEd25519
)

// ContextWithLocalID returns a context for local access.
func ContextWithLocalID(ctx Context) Context {
	return internal.ContextWithID(ctx, internal.LocalID)
}

// ContextID returns the principal id, if any.
func ContextID(ctx Context) *ID {
	return internal.ContextID(ctx)
}
