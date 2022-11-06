// Copyright (c) 2022 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package shell

import (
	"context"

	"gate.computer/gate/scope/program/system"
	"gate.computer/gate/service"
)

const (
	serviceName     = "gate.computer/shell"
	serviceRevision = "0"
)

type shell struct{}

func (shell) Properties() service.Properties {
	return service.Properties{
		Service: service.Service{
			Name:     serviceName,
			Revision: serviceRevision,
		},
		Streams: true,
	}
}

func (shell) Discoverable(ctx context.Context) bool {
	return system.ContextUserID(ctx) != ""
}

func (shell) CreateInstance(ctx context.Context, config service.InstanceConfig, snapshot []byte) (service.Instance, error) {
	inst := newInstance(config)
	inst.restore(snapshot)
	return inst, nil
}
