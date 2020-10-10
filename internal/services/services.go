// Copyright (c) 2020 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package services

import (
	"context"

	"gate.computer/gate/server"
	"gate.computer/gate/service"
	"gate.computer/gate/service/catalog"
	grpc "gate.computer/gate/service/grpc/config"
	"gate.computer/gate/service/identity"
	"gate.computer/gate/service/origin"
	"gate.computer/gate/service/plugin"
)

type Logger = grpc.Logger

func Init(ctx context.Context, plugins plugin.ServicePlugins, originConfig origin.Config, stderr Logger) (
	func(context.Context) server.InstanceServices,
	error,
) {
	registry := new(service.Registry)

	if err := plugins.InitServices(ctx, registry); err != nil {
		return nil, err
	}

	if err := grpc.InitServices(ctx, registry, stderr); err != nil {
		return nil, err
	}

	services := func(ctx context.Context) server.InstanceServices {
		o := origin.New(originConfig)

		r := registry.Clone()
		r.MustRegister(o)
		r.MustRegister(catalog.New(r))
		r.MustRegister(identity.Service)

		return server.NewInstanceServices(o, r)
	}

	return services, nil
}
