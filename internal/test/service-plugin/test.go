// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"log"

	"gate.computer/gate/packet"
	"gate.computer/gate/principal"
	"gate.computer/gate/service"
)

const (
	serviceName     = "internal/test"
	serviceRevision = "10.23.456-7"
)

var testConfig struct {
	MOTD string
}

func ServiceConfig() interface{} {
	return &testConfig
}

func InitServices(ctx context.Context, r *service.Registry) error {
	return r.Register(testService{})
}

type testService struct{}

func (testService) Service() service.Service {
	return service.Service{
		Name:     serviceName,
		Revision: serviceRevision,
	}
}

func (testService) Discoverable(ctx context.Context) bool {
	return principal.ContextID(ctx) != nil
}

func (testService) CreateInstance(ctx context.Context, config service.InstanceConfig, snapshot []byte) (service.Instance, error) {
	if snapshot == nil {
		log.Print(testConfig.MOTD)
	} else {
		log.Print(testConfig.MOTD, "again")
	}

	return testInstance{}, nil
}

type testInstance struct {
	service.InstanceBase
}

func (testInstance) Handle(ctx context.Context, replies chan<- packet.Buf, p packet.Buf) error {
	switch dom := p.Domain(); {
	case dom == packet.DomainCall:
		replies <- p

	case dom.IsStream():
		panic("unexpected stream packet")
	}

	return nil
}

func (testInstance) Suspend(ctx context.Context) ([]byte, error) {
	return []byte{0x73, 0x57}, nil
}

func main() {}
