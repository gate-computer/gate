// Copyright (c) 2025 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package identity

import (
	"testing"

	"github.com/google/uuid"
)

func TestPrincipalID(t *testing.T) {
	s := <-PrincipalID()
	if s != "local" {
		t.Errorf("principal ID: %q", s)
	}
}

func TestInstanceID(t *testing.T) {
	s := <-InstanceID()
	t.Logf("instance ID: %q", s)
	if _, err := uuid.Parse(s); err != nil {
		t.Error(err)
	}
}
