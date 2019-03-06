// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"log"

	"github.com/tsavola/gate/packet"
	"github.com/tsavola/gate/service"
)

const (
	ServiceName = "internal/test"
)

var testConfig struct {
	MOTD string
}

func ServiceConfig() interface{} {
	return &testConfig
}

func InitServices(config service.Config) (err error) {
	config.Registry.Register(testService{})
	return
}

type testService struct{}

func (testService) ServiceName() string {
	return ServiceName
}

func (testService) CreateInstance(service.InstanceConfig) service.Instance {
	log.Print(testConfig.MOTD)
	return testInstance{}
}

func (testService) RecreateInstance(service.InstanceConfig, []byte) (service.Instance, error) {
	log.Print(testConfig.MOTD, "again")
	return testInstance{}, nil
}

type testInstance struct{}

func (testInstance) Handle(ctx context.Context, replies chan<- packet.Buf, p packet.Buf) {
	switch p.Domain() {
	case packet.DomainCall:
		replies <- p
	}
}

func (testInstance) ExtractState() []byte {
	return []byte{0x73, 0x57}
}

func (testInstance) Close() (err error) {
	return
}

func main() {}
