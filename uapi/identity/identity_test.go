// Copyright (c) 2025 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package identity

import (
	"log/slog"
	"os"
	"testing"

	"github.com/google/uuid"
)

func init() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))
}

func TestPrincipalID(t *testing.T) {
	s := <-PrincipalID()
	if s != "local" {
		t.Errorf("principal ID: %q", s)
	}
}

func TestInstanceID(t *testing.T) {
	s := <-InstanceID()
	if _, err := uuid.Parse(s); err != nil {
		t.Logf("instance ID: %q", s)
		t.Error(err)
	}
}
