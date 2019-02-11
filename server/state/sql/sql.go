// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package sql implements an AccessTracker backed by an SQL database.
//
// Supports at least PostgreSQL 9.5+ (github.com/lib/pq) and SQLite 3.24+
// (github.com/mattn/go-sqlite3).
package sql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"time"

	"github.com/tsavola/gate/server"
	"github.com/tsavola/gate/server/state"
)

var errNonceExists = errors.New("nonce already exists")

func init() {
	state.Register("sql", adapter{})
}

const Schema = `
CREATE TABLE IF NOT EXISTS nonce (
	principal BYTEA NOT NULL,
	nonce TEXT NOT NULL,
	expire BIGINT NOT NULL,

	PRIMARY KEY (principal, nonce)
);

CREATE INDEX IF NOT EXISTS nonce_expire ON nonce (expire);
`

type Config struct {
	Driver string // Ignored if Connector is defined.
	DSN    string //

	Connector driver.Connector // Overrides Driver and DSN.
}

type adapter struct{}

func (adapter) NewConfig() interface{} {
	return new(Config)
}

func (adapter) Open(ctx context.Context, config interface{}) (state.DB, error) {
	return Open(ctx, *config.(*Config))
}

type DB struct {
	state.AccessTrackerBase
	db *sql.DB
}

func Open(ctx context.Context, config Config) (db *DB, err error) {
	db = new(DB)

	if config.Connector != nil {
		db.db = sql.OpenDB(config.Connector)
	} else {
		db.db, err = sql.Open(config.Driver, config.DSN)
		if err != nil {
			return
		}
	}
	defer func() {
		if err != nil {
			db.db.Close()
		}
	}()

	_, err = db.db.ExecContext(ctx, Schema)
	if err != nil {
		return
	}

	return
}

func (db *DB) Close() error {
	return db.db.Close()
}

func (db *DB) TrackNonce(ctx context.Context, pri *server.PrincipalKey, nonce string, expire time.Time,
) (err error) {
	conn, err := db.db.Conn(ctx)
	if err != nil {
		return
	}
	defer conn.Close()

	_, err = conn.ExecContext(ctx, "DELETE FROM nonce WHERE expire < $1", time.Now().Unix())
	if err != nil {
		return
	}

	result, err := conn.ExecContext(ctx, "INSERT INTO nonce (principal, nonce, expire) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING", pri.KeyBytes(), nonce, expire.Unix())
	if err != nil {
		return
	}

	n, err := result.RowsAffected()
	if err != nil {
		return
	}
	if n == 0 {
		err = errNonceExists
		return
	}

	return
}
