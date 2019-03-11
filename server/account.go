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

// cleanup must be called with Server.lock held.
func (acc *account) cleanup() (is map[string]*Instance) {
	ps := acc.programRefs
	acc.programRefs = nil

	for prog := range ps {
		prog.unref()
	}

	is = acc.instances
	acc.instances = nil
	return
}

// ensureRefProgram must be called with Server.lock held.  It's safe to call
// for an already referenced program.  It must not be called while the server
// is shutting down.
func (acc *account) ensureRefProgram(prog *program) {
	if _, exists := acc.programRefs[prog]; !exists {
		prog.ref()
		acc.programRefs[prog] = struct{}{}
	}
}

// unrefProgram must be called with Server.lock held.
func (acc *account) unrefProgram(prog *program) {
	if _, ok := acc.programRefs[prog]; !ok {
		panic("account does not reference program")
	}
	delete(acc.programRefs, prog)
	prog.unref()
}

// checkUniqueInstanceID must be called with Server.lock held.
func (acc *account) checkUniqueInstanceID(instID string) (err error) {
	if _, exists := acc.instances[instID]; exists {
		err = failrequest.New(event.FailRequest_InstanceIdExists, "duplicate instance id")
	}
	return
}
