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

func InitServices(ctx context.Context, r *service.Registry) (err error) {
	r.Register(testService{})
	return
}

type testService struct{}

func (testService) ServiceName() string {
	return serviceName
}

func (testService) ServiceRevision() string {
	return serviceRevision
}

func (testService) Discoverable(ctx context.Context) bool {
	return principal.ContextID(ctx) != nil
}

func (testService) CreateInstance(context.Context, service.InstanceConfig) service.Instance {
	log.Print(testConfig.MOTD)
	return testInstance{}
}

func (testService) RestoreInstance(context.Context, service.InstanceConfig, []byte) (service.Instance, error) {
	log.Print(testConfig.MOTD, "again")
	return testInstance{}, nil
}

type testInstance struct{}

func (testInstance) Resume(ctx context.Context, replies chan<- packet.Buf) {
}

func (testInstance) Handle(ctx context.Context, replies chan<- packet.Buf, p packet.Buf) {
	switch dom := p.Domain(); {
	case dom == packet.DomainCall:
		replies <- p

	case dom.IsStream():
		panic("unexpected stream packet")
	}
}

func (testInstance) Suspend(ctx context.Context) []byte {
	return []byte{0x73, 0x57}
}

func (testInstance) Shutdown(ctx context.Context) {
}

func main() {}
