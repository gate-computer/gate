// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"reflect"

	"gate.computer/gate/server/event"
	"gate.computer/gate/server/internal/error/failrequest"
	"gate.computer/internal/principal"
)

type accountProgram struct {
	tags []string
}

type accountInstance struct {
	inst *Instance
	prog *program
}

type account struct {
	*principal.ID

	// Protected by server mutex:
	programs  map[*program]accountProgram
	instances map[string]accountInstance
}

func newAccount(pri *principal.ID) *account {
	return &account{
		ID:        pri,
		programs:  make(map[*program]accountProgram),
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
func (acc *account) ensureProgramRef(lock serverLock, prog *program, tags []string) (modified bool) {
	x, found := acc.programs[prog]
	if !found {
		prog.ref(lock)
		modified = true
	}
	if len(tags) != 0 && !reflect.DeepEqual(x.tags, tags) {
		x.tags = append([]string(nil), tags...)
		modified = true
	}
	if modified {
		acc.programs[prog] = x
	}
	return
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

func (acc *account) _checkUniqueInstanceID(_ serverLock, id string) {
	if _, found := acc.instances[id]; found {
		_check(failrequest.Error(event.FailInstanceIDExists, "duplicate instance id"))
	}
}
