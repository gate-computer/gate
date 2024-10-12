// Copyright (c) 2024 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package serverapi

import (
	"context"

	pb "gate.computer/gate/pb/server"

	. "import.name/type/context"
)

type contextOpKey struct{}

var contextOp any = contextOpKey{}

func ContextWithOp(ctx Context, op pb.Op) Context {
	return context.WithValue(ctx, contextOp, op)
}

func ContextOp(ctx Context) pb.Op {
	op, _ := ctx.Value(contextOp).(pb.Op)
	return op
}
