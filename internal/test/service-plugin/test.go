// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"log"

	"github.com/tsavola/gate/packet"
	"github.com/tsavola/gate/principal"
	"github.com/tsavola/gate/service"
)

const (
	ServiceName    = "internal/test"
	ServiceVersion = "10.23.456-7"
)

var testConfig struct {
	MOTD string
}

func ServiceConfig() interface{} {
	return &testConfig
}

func InitServices(r *service.Registry) (err error) {
	r.Register(testService{})
	return
}

type testService struct{}

func (testService) ServiceName() string {
	return ServiceName
}

func (testService) ServiceVersion() string {
	return ServiceVersion
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
	switch p.Domain() {
	case packet.DomainCall:
		replies <- p
	}
}

func (testInstance) Suspend() []byte {
	return []byte{0x73, 0x57}
}

func (testInstance) Shutdown() {
}

func main() {}
