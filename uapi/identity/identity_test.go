// Copyright (c) 2025 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package identity

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	. "import.name/testing/mustr"
)

func TestPrincipalID(t *testing.T) {
	assert.Equal(t, <-PrincipalID(), "local")
}

func TestInstanceID(t *testing.T) {
	id := Must(t, R(uuid.Parse(<-InstanceID())))
	assert.Equal(t, id.Variant(), uuid.RFC4122)
	assert.Equal(t, id.Version(), uuid.Version(4))
}
