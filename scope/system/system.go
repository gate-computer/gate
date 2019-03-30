// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package system

import (
	"context"

	internal "github.com/tsavola/gate/internal/system"
)

func ContextUserID(ctx context.Context) string {
	s, _ := ctx.Value(internal.ContextUID).(string)
	return s
}
