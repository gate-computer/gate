// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package system

import (
	"context"
)

type contextKey int

const (
	ContextUID contextKey = iota
)

func ContextWithUserID(ctx context.Context, uid string) context.Context {
	if uid == "" {
		return ctx
	}

	return context.WithValue(ctx, ContextUID, uid)
}
