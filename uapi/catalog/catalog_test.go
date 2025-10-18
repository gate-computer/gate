// Copyright (c) 2025 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package catalog

import (
	"encoding/json"
	"log/slog"
	"os"
	"testing"
)

func init() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))
}

func TestJSON(t *testing.T) {
	b := <-JSON()

	if err := json.Unmarshal(b, new(any)); err != nil {
		t.Fatal(err)
	}

	t.Logf("%s", b)
}
