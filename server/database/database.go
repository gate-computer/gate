// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/tsavola/gate/server"
)

var ErrNonceReused = errors.New("nonce reused")

type NonceChecker interface {
	CheckNonce(ctx context.Context, pri *server.PrincipalKey, nonce string, expires time.Time) error
	Close() error
}

type Adapter struct {
	NewConfig        func() interface{}
	OpenNonceChecker func(ctx context.Context, config interface{}) (NonceChecker, error)
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

func OpenNonceChecker(ctx context.Context, name string, config interface{}) (db NonceChecker, err error) {
	a, found := adapters[name]
	if !found {
		err = fmt.Errorf("database adapter not registered: %s", name)
		return
	}
	if a.OpenNonceChecker == nil {
		err = fmt.Errorf("database adapter does not support nonce checking: %s", name)
		return
	}

	return a.OpenNonceChecker(ctx, config)
}
