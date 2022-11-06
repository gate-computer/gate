// Copyright (c) 2022 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package event

import (
	"gate.computer/gate/server/event/pb"
)

type (
	Event         = pb.Event
	EventFail     = pb.Event_Fail
	EventInstance = pb.Event_Instance
	EventModule   = pb.Event_Module
	Fail          = pb.Fail
	FailType      = pb.Fail_Type
	Instance      = pb.Instance
	Module        = pb.Module
	Type          = pb.Type
)
