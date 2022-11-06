// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"context"

	"gate.computer/internal/container"
	"gate.computer/internal/file"
)

type GroupProcessFactory interface {
	ProcessFactory
	NewGroupProcess(context.Context, *ProcessGroup) (*Process, error)
}

type ProcessGroup struct {
	dir file.Ref
}

func OpenCgroup(name string) (*ProcessGroup, error) {
	fd, err := container.OpenCgroupFD(name)
	if err != nil {
		return nil, err
	}

	return &ProcessGroup{file.Own(fd)}, nil
}

func (g *ProcessGroup) Close() error {
	g.dir.Unref()
	return nil
}
