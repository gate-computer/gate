// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package system

import (
	"fmt"
	"strconv"

	"gate.computer/gate/runtime"
	"gate.computer/gate/scope/program/system"

	. "import.name/type/context"
)

// GroupUserProcesses returns a factory which creates authenticated users'
// processes into their systemd slices.
func GroupUserProcesses(users runtime.GroupProcessFactory, other runtime.ProcessFactory) runtime.ProcessFactory {
	return userGrouper{users, other}
}

type userGrouper struct {
	users runtime.GroupProcessFactory
	other runtime.ProcessFactory
}

func (x userGrouper) NewProcess(ctx Context) (*runtime.Process, error) {
	userID := system.ContextUserID(ctx)
	if userID == "" {
		return x.other.NewProcess(ctx)
	}

	id, err := strconv.ParseUint(userID, 10, 31)
	if err != nil {
		return nil, err
	}

	slice, err := runtime.OpenCgroup(fmt.Sprintf("user.slice/user-%d.slice", id))
	if err != nil {
		return nil, err
	}
	defer slice.Close()

	return x.users.NewGroupProcess(ctx, slice)
}
