// Copyright (c) 2022 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"

	"import.name/pan"
)

// icky instance is passed between functions which may propagate errors via
// panic.  Only a function calling recover() should instantiate this type.
type icky struct{}

func (icky) check(err error)           { pan.Check(err) }
func (icky) error(x interface{}) error { return pan.Error(x) }
func (icky) wrap(err error) error      { return pan.Wrap(err) }

func (icky) mustContext(ctx context.Context, err error) context.Context {
	pan.Check(err)
	return ctx
}
