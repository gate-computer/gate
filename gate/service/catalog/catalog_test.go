// Copyright (c) 2025 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package catalog

import (
	"encoding/json"
	"testing"

	"gate.computer/gate/packet"
	"gate.computer/gate/service"
	"gate.computer/gate/service/identity"
	"gate.computer/gate/service/servicetest"
	"github.com/stretchr/testify/assert"
)

func newFactory() service.Factory {
	r := new(service.Registry)
	s := New(r)
	r.MustRegister(s)
	r.MustRegister(identity.Service)
	return s
}

func TestFactory(t *testing.T) {
	servicetest.FactoryTest(t.Context(), t, newFactory(), servicetest.FactorySpec{
		NoStreams:          true,
		AlwaysDiscoverable: true,
	})
}

func TestInstance(t *testing.T) {
	i := servicetest.NewInstanceTester(t.Context(), t, newFactory(), servicetest.InstanceSpec{})

	p := i.Handle(t.Context(), t, append(packet.MakeCall(servicetest.Code, 0), "json"...))

	var r map[string][]service.Service
	assert.NoError(t, json.Unmarshal(p.Content(), &r))
	assert.True(t, len(r) == 1)
	assert.ElementsMatch(t, r["services"], []service.Service{
		{Name: "catalog", Revision: "0"},
		{Name: "identity", Revision: "0"},
	})

	i.Shutdown(t.Context(), t)
}
