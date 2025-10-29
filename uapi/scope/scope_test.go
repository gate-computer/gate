// Copyright (c) 2025 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package scope

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRestrict(t *testing.T) {
	assert.NoError(t, <-Restrict([]string{System}))
	assert.NoError(t, <-Restrict([]string{System}))
	assert.NoError(t, <-Restrict(nil))
	assert.NoError(t, <-Restrict(nil))
	assert.NoError(t, <-Restrict([]string{"ignored"}))
}
