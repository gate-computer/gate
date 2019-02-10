// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webserver

import (
	"github.com/tsavola/gate/server"
	"github.com/tsavola/gate/service"
	"github.com/tsavola/gate/service/origin"
	"github.com/tsavola/gate/service/plugin"
)

func newTestServices() func() server.InstanceServices {
	registry := new(service.Registry)

	plugins, err := plugin.OpenAll("../../lib/gate/plugin")
	if err != nil {
		panic(err)
	}

	err = plugins.InitServices(service.Config{
		Registry: registry,
	})
	if err != nil {
		panic(err)
	}

	return func() server.InstanceServices {
		connector := origin.New(nil)
		r := registry.Clone()
		r.Register(connector)
		return server.NewInstanceServices(r, connector)
	}
}
