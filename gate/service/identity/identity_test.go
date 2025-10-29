// Copyright (c) 2025 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package identity

import (
	"testing"

	"gate.computer/gate/packet"
	"gate.computer/gate/principal"
	"gate.computer/gate/service/servicetest"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestFactory(t *testing.T) {
	servicetest.FactoryTest(t.Context(), t, Service, servicetest.FactorySpec{
		NoStreams:          true,
		AlwaysDiscoverable: true,
	})
}

func TestInstance(t *testing.T) {
	instanceID := uuid.New()

	ctx := t.Context()
	ctx = principal.ContextWithLocalID(ctx)
	ctx = principal.ContextWithInstanceUUID(ctx, instanceID)

	i := servicetest.NewInstanceTester(ctx, t, Service, servicetest.InstanceSpec{})

	p := i.Handle(ctx, t, append(packet.MakeCall(servicetest.Code, 0), callPrincipalID))
	assert.Equal(t, string(p.Content()), "local")

	p = i.Handle(ctx, t, append(packet.MakeCall(servicetest.Code, 0), callInstanceID))
	assert.Equal(t, string(p.Content()), instanceID.String())

	i.Shutdown(ctx, t)
}
