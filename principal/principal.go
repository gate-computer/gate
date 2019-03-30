// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package principal

import (
	"context"

	internal "github.com/tsavola/gate/internal/principal"
)

type ID = internal.ID
type Key = internal.Key

// ContextID returns the principal id, if any.
func ContextID(ctx context.Context) (id ID, ok bool) {
	id, ok = ctx.Value(internal.ContextID).(ID)
	return
}
