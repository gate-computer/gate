// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"log/slog"

	"gate.computer/gate/packet"
	"gate.computer/gate/principal"
	"gate.computer/gate/service"
	"gate.computer/gate/service/logger"

	. "import.name/type/context"
)

const (
	extName         = "text"
	serviceName     = "internal/test"
	serviceRevision = "10.23.456-7"
)

var testConfig struct {
	MOTD string
}

var Ext = service.Extend(extName, &testConfig, func(ctx Context, r *service.Registry) error {
	return r.Register(testService{logger.MustContextual(ctx)})
})

type testService struct {
	log *slog.Logger
}

func (testService) Properties() service.Properties {
	return service.Properties{
		Service: service.Service{
			Name:     serviceName,
			Revision: serviceRevision,
		},
	}
}

func (testService) Discoverable(ctx Context) bool {
	return principal.ContextID(ctx) != nil
}

func (s testService) CreateInstance(ctx Context, config service.InstanceConfig, snapshot []byte) (service.Instance, error) {
	if snapshot == nil {
		s.log.InfoContext(ctx, testConfig.MOTD)
	} else {
		s.log.InfoContext(ctx, testConfig.MOTD, "snapshot", true)
	}

	return testInstance{}, nil
}

type testInstance struct {
	service.InstanceBase
}

func (testInstance) Handle(ctx Context, replies chan<- packet.Thunk, p packet.Buf) (packet.Buf, error) {
	if p.Domain() == packet.DomainCall {
		return p, nil
	}

	return nil, nil
}

func (testInstance) Suspend(ctx Context) ([]byte, error) {
	return []byte{0x73, 0x57}, nil
}
