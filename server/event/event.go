// Copyright (c) 2022 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package event

import (
	"gate.computer/gate/server/event/pb"
)

type Event = pb.Event
type EventFail = pb.Event_Fail
type EventInstance = pb.Event_Instance
type EventModule = pb.Event_Module
type Fail = pb.Fail
type FailType = pb.Fail_Type
type Instance = pb.Instance
type Module = pb.Module
type Type = pb.Type
