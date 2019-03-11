// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

type Storage interface {
	ProgramStorage
	InstanceStorage

	LoadProgram(name string) (*Program, error)

	singleBackend() bool
}

func CombinedStorage(prog ProgramStorage, inst InstanceStorage) Storage {
	if prog.(interface{}) == inst.(interface{}) {
		return prog.(Storage)
	} else {
		return &combinedStorage{prog, inst, prog.programBackend() == inst.instanceBackend()}
	}
}

type combinedStorage struct {
	ProgramStorage
	InstanceStorage
	sameBackend bool
}

func (cs *combinedStorage) LoadProgram(name string) (prog *Program, err error) {
	return cs.loadProgram(cs, name)
}

func (cs *combinedStorage) singleBackend() bool {
	return cs.sameBackend
}
