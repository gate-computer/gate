// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package badrequest

import (
	"github.com/tsavola/gate/internal/error/public"
)

type Error interface {
	public.Error
	BadRequest()
}
