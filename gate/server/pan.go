// Copyright (c) 2022 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"import.name/pan"

	. "import.name/type/context"
)

// icky instance is passed between functions which may propagate errors via
// panic.  Only a function calling recover() should instantiate this type.
type icky struct{}

func (icky) check(err error)      { pan.Check(err) }
func (icky) error(x any) error    { return pan.Error(x) }
func (icky) wrap(err error) error { return pan.Wrap(err) }

func (icky) mustContext(ctx Context, err error) Context {
	pan.Check(err)
	return ctx
}
