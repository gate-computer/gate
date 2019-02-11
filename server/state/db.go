// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package state

import (
	"context"
	"fmt"
	"io"
)

// DB implementations have same restrictions as AccessTracker implementations.
type DB interface {
	AccessTracker
	io.Closer
}

type Adapter interface {
	NewConfig() interface{}
	Open(ctx context.Context, config interface{}) (DB, error)
}

var (
	adapters      = make(map[string]Adapter)
	DefaultConfig = make(map[string]interface{})
)

func Register(name string, a Adapter) {
	if _, exists := adapters[name]; exists {
		panic(fmt.Errorf("database adapter already registered: %s", name))
	}

	adapters[name] = a
	DefaultConfig[name] = a.NewConfig()
}

func NewConfig(name string) (interface{}, error) {
	a, found := adapters[name]
	if !found {
		return nil, fmt.Errorf("database adapter not registered: %s", name)
	}

	return a.NewConfig(), nil
}

func Open(ctx context.Context, name string, config interface{}) (db DB, err error) {
	a, found := adapters[name]
	if !found {
		err = fmt.Errorf("database adapter not registered: %s", name)
		return
	}

	return a.Open(ctx, config)
}
