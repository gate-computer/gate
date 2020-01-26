// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"github.com/tsavola/gate/principal"
	"github.com/tsavola/gate/server/event"
	"github.com/tsavola/gate/server/internal/error/failrequest"
)

type accountInstance struct {
	inst *Instance
	prog *program
}

type account struct {
	*principal.ID

	// Protected by server mutex:
	programs  map[*program]struct{}
	instances map[string]accountInstance
}

func newAccount(pri *principal.ID) *account {
	return &account{
		ID:        pri,
		programs:  make(map[*program]struct{}),
		instances: make(map[string]accountInstance),
	}
}

func (acc *account) shutdown(lock serverLock) (is map[string]accountInstance) {
	ps := acc.programs
	acc.programs = nil

	for prog := range ps {
		prog.unref(lock)
	}

	is = acc.instances
	acc.instances = nil
	return
}

// ensureProgramRef adds program reference unless already found.  It must not
// be called while the server is shutting down.
func (acc *account) ensureProgramRef(lock serverLock, prog *program) {
	if _, exists := acc.programs[prog]; !exists {
		acc.programs[prog.ref(lock)] = struct{}{}
	}
}

// refProgram if found.
func (acc *account) refProgram(lock serverLock, prog *program) *program {
	if _, found := acc.programs[prog]; found {
		return prog.ref(lock)
	}
	return nil
}

// unrefProgram if found.
func (acc *account) unrefProgram(lock serverLock, prog *program) (found bool) {
	_, found = acc.programs[prog]
	if found {
		delete(acc.programs, prog)
		prog.unref(lock)
	}
	return
}

func (acc *account) checkUniqueInstanceID(_ serverLock, instID string) error {
	if _, exists := acc.instances[instID]; exists {
		return failrequest.New(event.FailInstanceIDExists, "duplicate instance id")
	}
	return nil
}
