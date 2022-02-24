// Copyright (c) 2020 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package services

import (
	"context"

	"gate.computer/gate/server"
	"gate.computer/gate/service"
	"gate.computer/gate/service/catalog"
	"gate.computer/gate/service/identity"
	"gate.computer/gate/service/origin"
	"gate.computer/gate/service/random"
	"gate.computer/gate/service/scope"
)

func Init(ctx context.Context, originConfig *origin.Config, randomConfig *random.Config) (
	func(context.Context) server.InstanceServices,
	error,
) {
	registry := new(service.Registry)

	if err := service.Init(ctx, registry); err != nil {
		return nil, err
	}

	services := func(ctx context.Context) server.InstanceServices {
		o := origin.New(originConfig)

		r := registry.Clone()
		r.MustRegister(o)
		r.MustRegister(catalog.New(r))
		r.MustRegister(identity.Service)
		r.MustRegister(random.New(randomConfig))
		r.MustRegister(scope.Service)

		return server.NewInstanceServices(o, r)
	}

	return services, nil
}
