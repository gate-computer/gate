// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webserver

import (
	"context"
	"time"

	"github.com/tsavola/gate/server"
)

// AccessTracker may be expanded with new methods (prefixed with the Track
// namespace) also between major releases.  Implementations must inherit
// methods from the AccessTrackerBase, and must not add unrelated methods with
// the Track prefix to avoid breakage.
//
// A method won't be added (in a minor release) unless there can be a
// universally applicable fallback implementation for it.
type AccessTracker interface {
	// TrackNonce returns an error when it sees a nonce from the same principal
	// before the previous occurences have expired.
	TrackNonce(ctx context.Context, pri *server.PrincipalKey, nonce string, expires time.Time) error

	accessTracker() // Force inheritance.
}

// AccessTrackerBase will add fallback implementations of future AccessTracker
// methods.
type AccessTrackerBase struct{}

func (AccessTrackerBase) accessTracker() {}
