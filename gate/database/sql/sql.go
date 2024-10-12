// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package sql implements server [model] interfaces.  Supports at least SQLite
// and PostgreSQL.
package sql

import (
	"database/sql"
	"strings"

	"gate.computer/gate/database"
	"gate.computer/gate/server/model"

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
	db     *sql.DB
	driver string
}

func Open(config Config) (*Endpoint, error) {
	db, err := sql.Open(config.Driver, config.DSN)
	if err != nil {
		return nil, err
	}
	return &Endpoint{db, config.Driver}, nil
}

func (x *Endpoint) Close() error {
	return x.db.Close()
}

func (x *Endpoint) adjustSchema(s string) string {
	switch x.driver {
	case "sqlite", "sqlite3":
		s = strings.ReplaceAll(s, " BIGINT", " INTEGER")

	default:
		s = strings.ReplaceAll(s, " WITHOUT ROWID, STRICT;", ";")
	}

	return s
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

	InitInventory: func(ctx Context, endpoint database.Endpoint) (model.Inventory, error) {
		x := endpoint.(*Endpoint)
		if err := x.InitInventory(ctx); err != nil {
			return nil, err
		}
		return x, nil
	},

	InitSourceCache: func(ctx Context, endpoint database.Endpoint) (model.SourceCache, error) {
		x := endpoint.(*Endpoint)
		if err := x.InitSourceCache(ctx); err != nil {
			return nil, err
		}
		return x, nil
	},

	InitNonceChecker: func(ctx Context, endpoint database.Endpoint) (model.NonceChecker, error) {
		x := endpoint.(*Endpoint)
		if err := x.InitNonceChecker(ctx); err != nil {
			return nil, err
		}
		return x, nil
	},
})
