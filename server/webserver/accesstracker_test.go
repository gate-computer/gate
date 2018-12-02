// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webserver

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/tsavola/gate/server"
)

type testAccessTracker struct {
	AccessTrackerBase

	lock sync.Mutex
	m    map[string]struct{}
}

func newTestAccessTracker() *testAccessTracker {
	return &testAccessTracker{
		m: make(map[string]struct{}),
	}
}

func (at *testAccessTracker) TrackNonce(ctx context.Context, pri *server.PrincipalKey, nonce string, expires time.Time) (err error) {
	key := string(pri.KeyBytes()) + " " + nonce

	at.lock.Lock()
	defer at.lock.Unlock()

	if _, found := at.m[key]; found {
		err = errors.New("duplicate nonce")
	} else {
		at.m[key] = struct{}{}
	}
	return
}
