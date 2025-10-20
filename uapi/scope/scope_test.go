// Copyright (c) 2025 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package scope

import (
	"testing"
)

func TestRestrict(t *testing.T) {
	if err := <-Restrict([]string{System}); err != nil {
		t.Fatal(err)
	}
	if err := <-Restrict([]string{System}); err != nil {
		t.Fatal(err)
	}
	if err := <-Restrict(nil); err != nil {
		t.Fatal(err)
	}
	if err := <-Restrict(nil); err != nil {
		t.Fatal(err)
	}
	if err := <-Restrict([]string{"ignored"}); err != nil {
		t.Fatal(err)
	}
}
