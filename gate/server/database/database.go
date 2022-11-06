// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package database

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"
)

type Inventory interface {
	GetSourceModule(ctx context.Context, source string) (module string, err error)
	AddModuleSource(ctx context.Context, module, source string) error
}

var ErrNonceReused = errors.New("nonce reused")

type NonceChecker interface {
	CheckNonce(ctx context.Context, scope []byte, nonce string, expires time.Time) error
}

type Config interface {
	Enabled() bool
	Equal(Config) bool
}

type Endpoint interface {
	io.Closer
}

type Adapter struct {
	Name             string
	NewConfig        func() Config
	Open             func(Config) (Endpoint, error)
	InitInventory    func(context.Context, Endpoint) (Inventory, error)
	InitNonceChecker func(context.Context, Endpoint) (NonceChecker, error)
}

func (a *Adapter) String() string {
	return a.Name
}

var adapters = make(map[string]*Adapter)

func Adapters() map[string]*Adapter {
	return adapters
}

func Register(a *Adapter) *Adapter {
	if _, exists := adapters[a.Name]; exists {
		panic(fmt.Errorf("database adapter already registered: %s", a.Name))
	}
	adapters[a.Name] = a
	return a
}

func getAdapter(name string) (*Adapter, error) {
	a, found := adapters[name]
	if !found {
		return nil, fmt.Errorf("database adapter not registered: %s", name)
	}
	return a, nil
}

func NewInventoryConfigs() map[string]Config {
	c := make(map[string]Config, len(adapters))
	for _, a := range adapters {
		if a.InitInventory != nil {
			c[a.Name] = a.NewConfig()
		}
	}
	return c
}

func NewNonceCheckerConfigs() map[string]Config {
	c := make(map[string]Config, len(adapters))
	for _, a := range adapters {
		if a.InitNonceChecker != nil {
			c[a.Name] = a.NewConfig()
		}
	}
	return c
}

var ErrNoConfig = errors.New("no database configuration")

func resolveConfig(configs map[string]Config) (string, Config, error) {
	var firstName string
	var firstConf Config

	for name, conf := range configs {
		if !conf.Enabled() {
			continue
		}
		if firstConf != nil {
			return "", nil, fmt.Errorf("multiple database configurations (%s and %s)", firstName, name)
		}
		firstName = name
		firstConf = conf
	}

	if firstConf == nil {
		return "", nil, ErrNoConfig
	}

	return firstName, firstConf, nil
}

type DB struct {
	Adapter *Adapter
	Config  Config

	refs     int
	endpoint Endpoint
}

var databases []*DB

func Resolve(configs map[string]Config) (*DB, error) {
	name, c, err := resolveConfig(configs)
	if err != nil {
		return nil, err
	}

	return Open(name, c)
}

func Open(name string, config Config) (*DB, error) {
	for _, db := range databases {
		if db.Adapter.Name == name && db.Config.Equal(config) {
			db.refs++
			return db, nil
		}
	}

	a, err := getAdapter(name)
	if err != nil {
		return nil, err
	}

	db := &DB{Adapter: a, Config: config, refs: 1}
	databases = append(databases, db)
	return db, nil
}

func (db *DB) get() (Endpoint, error) {
	if db.endpoint != nil {
		return db.endpoint, nil
	}

	x, err := db.Adapter.Open(db.Config)
	if err != nil {
		return nil, err
	}
	db.endpoint = x
	return x, nil
}

func (db *DB) Close() error {
	db.refs--
	if db.refs != 0 {
		return nil
	}

	x := db.endpoint
	if x == nil {
		return nil
	}
	db.endpoint = nil
	return x.Close()
}

func (db *DB) InitInventory(ctx context.Context) (Inventory, error) {
	x, err := db.get()
	if err != nil {
		return nil, err
	}

	return db.Adapter.InitInventory(ctx, x)
}

func (db *DB) InitNonceChecker(ctx context.Context) (NonceChecker, error) {
	x, err := db.get()
	if err != nil {
		return nil, err
	}

	return db.Adapter.InitNonceChecker(ctx, x)
}
