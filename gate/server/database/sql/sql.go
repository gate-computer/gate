// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package sql implements Inventory and NonceChecker backed by SQL database.
//
// Supports at least PostgreSQL 9.5+ (github.com/lib/pq) and
// SQLite 3.24+ (modernc.org/sqlite, github.com/mattn/go-sqlite3).
package sql

import (
	"database/sql"

	"gate.computer/gate/server/database"

	. "import.name/type/context"
)

type Config struct {
	Driver string
	DSN    string
}

func (c *Config) Enabled() bool {
	return c.Driver != "" && c.DSN != ""
}

func (c *Config) Equal(other Config) bool {
	return c.Driver == other.Driver && c.DSN == other.DSN
}

func (c *Config) Clone() Config {
	return *c
}

type adaptedConfig struct {
	Config
}

func (c *adaptedConfig) Equal(other database.Config) bool {
	return c.Config.Equal(other.(*adaptedConfig).Config)
}

type Endpoint struct {
	db *sql.DB
}

func Open(config Config) (*Endpoint, error) {
	db, err := sql.Open(config.Driver, config.DSN)
	if err != nil {
		return nil, err
	}
	return &Endpoint{db}, nil
}

func (x *Endpoint) Close() error {
	return x.db.Close()
}

var DefaultConfig Config

var Adapter = database.Register(&database.Adapter{
	Name: "sql",

	NewConfig: func() database.Config {
		return &adaptedConfig{
			Config: DefaultConfig.Clone(),
		}
	},

	Open: func(config database.Config) (database.Endpoint, error) {
		x, err := Open(config.(*adaptedConfig).Config)
		if err != nil {
			return nil, err
		}
		return x, err
	},

	InitInventory: func(ctx Context, endpoint database.Endpoint) (database.Inventory, error) {
		x := endpoint.(*Endpoint)
		if err := x.InitInventory(ctx); err != nil {
			return nil, err
		}
		return x, nil
	},

	InitNonceChecker: func(ctx Context, endpoint database.Endpoint) (database.NonceChecker, error) {
		x := endpoint.(*Endpoint)
		if err := x.InitNonceChecker(ctx); err != nil {
			return nil, err
		}
		return x, nil
	},
})
