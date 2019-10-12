// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package notapplicable

import (
	"github.com/tsavola/gate/server/event"
)

type Error interface {
	error
	NotApplicable()
	FailRequestType() event.FailRequest_Type
}

var ErrInstanceTransient err

type err struct{}

func (err) Error() string                           { return "instance is not persistent" }
func (err) PublicError() string                     { return "instance is not persistent" }
func (err) NotApplicable()                          {}
func (err) FailRequestType() event.FailRequest_Type { return event.FailInstanceTransient }
