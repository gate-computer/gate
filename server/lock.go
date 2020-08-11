// Copyright (c) 2020 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"sync"
)

type serverLock struct{}
type serverMutex struct{ sync.Mutex }

func (m *serverMutex) Lock() serverLock {
	m.Mutex.Lock()
	return serverLock{}
}

func (m *serverMutex) Guard(f func(serverLock)) {
	lock := m.Lock()
	defer m.Unlock()
	f(lock)
}

func (m *serverMutex) GuardBool(f func(serverLock) bool) bool {
	lock := m.Lock()
	defer m.Unlock()
	return f(lock)
}

func (m *serverMutex) GuardProgram(f func(serverLock) *program) *program {
	lock := m.Lock()
	defer m.Unlock()
	return f(lock)
}

type instanceLock struct{}
type instanceMutex struct{ sync.Mutex }

func (m *instanceMutex) Lock() instanceLock {
	m.Mutex.Lock()
	return instanceLock{}
}

func (m *instanceMutex) Guard(f func(instanceLock)) {
	lock := m.Lock()
	defer m.Unlock()
	f(lock)
}
