// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"github.com/tsavola/gate/server/event"
	"github.com/tsavola/gate/server/internal/error/failrequest"
)

func wrapContentError(err error) error {
	return failrequest.Wrap(event.FailPayloadError, err, "content decode error")
}
