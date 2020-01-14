// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"

	"github.com/tsavola/gate/principal"
	"github.com/tsavola/gate/server/detail"
	"github.com/tsavola/gate/server/event"
	"github.com/tsavola/gate/server/internal/error/failrequest"
)

func accountContext(ctx context.Context, acc *account) detail.Context {
	var pri *principal.ID
	if acc != nil {
		pri = acc.ID
	}
	return Context(ctx, pri)
}

type accountInstance struct {
	inst *Instance
	prog *program
}

type account struct {
	*principal.ID

	// Protected by server mutex:
	programRefs map[*program]struct{}
	instances   map[string]accountInstance
}

func newAccount(pri *principal.ID) *account {
	return &account{
		ID:          pri,
		programRefs: make(map[*program]struct{}),
		instances:   make(map[string]accountInstance),
	}
}

func (acc *account) cleanup(lock serverLock) (is map[string]accountInstance) {
	ps := acc.programRefs
	acc.programRefs = nil

	for prog := range ps {
		prog.unref(lock)
	}

	is = acc.instances
	acc.instances = nil
	return
}

// ensureRefProgram is safe to call for an already referenced program.  It must
// not be called while the server is shutting down.
func (acc *account) ensureRefProgram(lock serverLock, prog *program) {
	if _, exists := acc.programRefs[prog]; !exists {
		prog.ref(lock)
		acc.programRefs[prog] = struct{}{}
	}
}

func (acc *account) unrefProgram(lock serverLock, prog *program) {
	if _, ok := acc.programRefs[prog]; !ok {
		panic("account does not reference program")
	}
	delete(acc.programRefs, prog)
	prog.unref(lock)
}

func (acc *account) checkUniqueInstanceID(_ serverLock, instID string) (err error) {
	if _, exists := acc.instances[instID]; exists {
		err = failrequest.New(event.FailInstanceIDExists, "duplicate instance id")
	}
	return
}
