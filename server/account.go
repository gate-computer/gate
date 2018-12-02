// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"

	"github.com/tsavola/gate/server/detail"
	"github.com/tsavola/gate/server/event"
	"github.com/tsavola/gate/server/internal/error/failrequest"
)

func accountContext(ctx context.Context, acc *account) detail.Context {
	var pri *PrincipalKey
	if acc != nil {
		pri = acc.PrincipalKey
	}
	return Context(ctx, pri)
}

type account struct {
	*PrincipalKey

	// Protected by Server.lock:
	programRefs map[*program]struct{}
	instances   map[string]*Instance
}

func newAccount(pri *PrincipalKey) *account {
	return &account{
		PrincipalKey: pri,
		programRefs:  make(map[*program]struct{}),
		instances:    make(map[string]*Instance),
	}
}

func (acc *account) checkUniqueInstanceID(instID string) (err error) {
	if _, exists := acc.instances[instID]; exists {
		err = failrequest.New(event.FailRequest_InstanceIdExists, "duplicate instance id")
	}
	return
}
